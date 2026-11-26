package grader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type ContestJudge interface {
	Judge(ctx context.Context, submissionID string, req Request) (string, error)
}

type RemoteContestJudge struct {
	endpoint string
	token    string
	http     *http.Client
}

func NewRemoteContestJudge(endpoint, token string) *RemoteContestJudge {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &RemoteContestJudge{endpoint, token, &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

var errContestJudge = errors.New("Official judge unavailable or returned an invalid verdict.")

func (b *RemoteContestJudge) Judge(ctx context.Context, submissionID string, req Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", errContestJudge
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(data))
	if err != nil {
		return "", errContestJudge
	}
	request.Header.Set("Authorization", "Bearer "+b.token)
	request.Header.Set("Idempotency-Key", submissionID)
	request.Header.Set("Content-Type", "application/json")
	resp, err := b.http.Do(request)
	if err != nil {
		return "", errContestJudge
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errContestJudge
	}
	const maxVerdict = 4096
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxVerdict+1))
	if err != nil || len(payload) > maxVerdict {
		return "", errContestJudge
	}
	var verdict struct {
		Verdict string `json:"verdict"`
		Passed  *int   `json:"passed"`
		Total   int    `json:"total"`
	}
	if err := json.Unmarshal(payload, &verdict); err != nil || verdict.Passed == nil ||
		verdict.Total != len(req.Cases) || verdict.Total < 1 ||
		*verdict.Passed < 0 || *verdict.Passed > verdict.Total {
		return "", errContestJudge
	}
	switch verdict.Verdict {
	case "Accepted":
		if *verdict.Passed == verdict.Total {
			return verdict.Verdict, nil
		}
	case "Wrong Answer", "Compilation Error", "Runtime Error", "Time Limit Exceeded",
		"Memory Limit Exceeded", "Output Limit Exceeded":
		if *verdict.Passed < verdict.Total {
			return verdict.Verdict, nil
		}
	}
	return "", errContestJudge
}
