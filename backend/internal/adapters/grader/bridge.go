package grader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"vedam/lms/internal/domain/errs"
)

type Case struct {
	Input    string  `json:"input"`
	Expected *string `json:"expected"`
	Hidden   bool    `json:"hidden"`
}

type Request struct {
	Language   string `json:"language"`
	Source     string `json:"source"`
	Entrypoint string `json:"entrypoint"`
	Cases      []Case `json:"testCases"`
}

type CaseResult struct {
	Index      int             `json:"index"`
	Hidden     bool            `json:"hidden"`
	Status     string          `json:"status"`
	DurationMS int64           `json:"durationMs"`
	Input      *string         `json:"input,omitempty"`
	Expected   json.RawMessage `json:"expected,omitempty"`
	Stdout     *string         `json:"stdout,omitempty"`
	Stderr     *string         `json:"stderr,omitempty"`
}

type Result struct {
	Verdict      string       `json:"verdict"`
	Passed       int          `json:"passed"`
	Total        int          `json:"total"`
	CompileError string       `json:"compileError,omitempty"`
	Results      []CaseResult `json:"results"`
	AttemptID    string       `json:"attemptId,omitempty"`
}

type Unavailable struct{ Message string }

func (e Unavailable) Error() string { return e.Message }

type BusyError struct{ Message string }

func (e BusyError) Error() string { return e.Message }

// Evaluator defines the consumer-segregated port for sandboxed code grading (DIP).
type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (Result, error)
}

// Binary adapts the compiled Rust `lms-grader` executable via JSON-over-stdin/stdout.
type Binary struct{ Path string }

func (b Binary) Evaluate(ctx context.Context, req Request) (Result, error) {
	if b.Path == "" {
		return Result{}, Unavailable{"Sandboxed grader binary path is not configured."}
	}
	data, err := json.Marshal(req)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, b.Path)
	cmd.Stdin = bytes.NewReader(data)
	raw, err := cmd.Output()
	if err != nil {
		return Result{}, fmt.Errorf("start sandboxed grader (%s): %w", b.Path, err)
	}
	var response struct {
		Result  Result `json:"result"`
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return Result{}, fmt.Errorf("decode grader result: %w", err)
	}
	switch response.Status {
	case 0:
	case 400:
		return Result{}, errs.New(errs.ErrValidation, "INVALID_CODE_REQUEST", response.Message)
	case 429:
		return Result{}, BusyError{Message: response.Message}
	default:
		return Result{}, Unavailable{Message: response.Message}
	}
	result := response.Result
	if result.Verdict == "" || result.Total != len(req.Cases) || len(result.Results) > len(req.Cases) {
		return Result{}, fmt.Errorf("invalid grader response")
	}
	for i := range result.Results {
		if req.Cases[i].Hidden {
			row := &result.Results[i]
			row.Hidden = true
			row.Input, row.Expected, row.Stdout, row.Stderr = nil, nil, nil, nil
		}
	}
	return result, nil
}
