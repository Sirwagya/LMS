package grader_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vedam/lms/internal/adapters/grader"
)

func strPtr(s string) *string { return &s }

func TestBinaryEvaluator_PythonTwoSum(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := os.Getenv("GRADER_BINARY_PATH")
	if bin == "" {
		bin = filepath.Join(root, "target", "release", "lms-grader")
		if _, statErr := os.Stat(bin); statErr != nil {
			bin = filepath.Join(root, "target", "debug", "lms-grader")
		}
	}
	if _, statErr := os.Stat(bin); statErr != nil {
		t.Skipf("lms-grader binary not built at %s", bin)
	}

	var eval grader.Evaluator = grader.Binary{Path: bin}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	res, err := eval.Evaluate(ctx, grader.Request{
		Language:   "python",
		Entrypoint: "twoSum",
		Source:     "class Solution:\n    def twoSum(self, nums: list[int], target: int) -> list[int]:\n        seen={}\n        for i,x in enumerate(nums):\n            if target-x in seen:\n                return [seen[target-x], i]\n            seen[x]=i",
		Cases: []grader.Case{
			{Input: "4 9\n2 7 11 15\n", Expected: strPtr("0 1"), Hidden: false},
			{Input: "2 6\n3 3\n", Expected: strPtr("0 1"), Hidden: true},
		},
	})
	if err != nil {
		t.Fatalf("evaluate python twoSum: %v", err)
	}
	if res.Verdict != "Accepted" || res.Passed != 2 || res.Total != 2 {
		t.Fatalf("unexpected verdict: %+v", res)
	}
	if !res.Results[1].Hidden || res.Results[1].Input != nil || res.Results[1].Stdout != nil {
		t.Fatalf("hidden test details leaked: %+v", res.Results[1])
	}
}
