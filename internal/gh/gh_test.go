package gh

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func withStubbedGh(t *testing.T, runner ghRunnerFunc) func() {
	t.Helper()
	origRunner := ghCommandRunner
	origRepo := repositoryInfoFunc
	origCheck := checkGhAvailable

	ghCommandRunner = runner
	repositoryInfoFunc = func() (string, string, error) {
		return "owner", "repo", nil
	}
	checkGhAvailable = func() error { return nil }

	return func() {
		ghCommandRunner = origRunner
		repositoryInfoFunc = origRepo
		checkGhAvailable = origCheck
	}
}

func TestAddSubIssueSuccess(t *testing.T) {
	call := 0
	restore := withStubbedGh(t, func(ctx context.Context, args []string) (string, string, error) {
		call++
		if call == 1 {
			if !strings.Contains(strings.Join(args, " "), "issues/123") {
				t.Fatalf("expected first call to fetch issue id, got args: %v", args)
			}
			return "9999\n", "", nil
		}
		if !strings.Contains(strings.Join(args, " "), "sub_issues") {
			t.Fatalf("expected second call to add sub issue, got args: %v", args)
		}
		return "", "", nil
	})
	defer restore()

	if err := AddSubIssue("123", "10"); err != nil {
		t.Fatalf("AddSubIssue returned error: %v", err)
	}

	if call != 2 {
		t.Fatalf("expected two gh calls, got %d", call)
	}
}

func TestAddSubIssueMapsErrors(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantSubstr string
	}{
		{"notFound", "404 Not Found", "parent issue not found"},
		{"validation", "422 Validation Failed", "validation failed"},
		{"auth", "Requires authentication", "authentication required"},
		{"forbidden", "403 Forbidden", "permission denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := 0
			restore := withStubbedGh(t, func(ctx context.Context, args []string) (string, string, error) {
				call++
				if call == 1 {
					return "9999", "", nil
				}
				return "", tt.stderr, fmt.Errorf("exit")
			})
			defer restore()

			err := AddSubIssue("5", "1")
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
