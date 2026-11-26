package grader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

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
	raw, err := b.run(ctx, data)
	if err != nil {
		return Result{}, err
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

// Keep the judge slot occupied until the supervisor exits and its process group
// is terminated on cancellation. Rust handles SIGTERM first so it can reap the
// sandbox child itself. The group fallback covers a wedged supervisor.
func (b Binary) run(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "lms-grade-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command(b.Path)
	cmd.Stdin = bytes.NewReader(data)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(cmd.Environ(), "TMPDIR="+tmp, "LMS_GRADER_MANAGED_GROUP=1")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sandboxed grader: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			// An abnormal supervisor exit can leave a child in the same group.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return nil, fmt.Errorf("sandboxed grader exited: %w", err)
		}
		return output.Bytes(), nil
	case <-ctx.Done():
		_ = cmd.Process.Signal(syscall.SIGTERM)
		grace := time.NewTimer(2 * time.Second)
		defer grace.Stop()
		select {
		case <-done:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-grace.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return nil, ctx.Err()
	}
}
