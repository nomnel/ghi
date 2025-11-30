package gh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func helperCommandContext(t *testing.T) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()

	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
}

func stubGH(t *testing.T) func() {
	t.Helper()

	originalCommandContext := commandContext
	originalLookPath := commandLookPath
	originalRepoInfoFunc := repositoryInfoFunc

	commandContext = helperCommandContext(t)
	commandLookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	repositoryInfoFunc = func() (string, string, error) { return "owner", "repo", nil }

	return func() {
		commandContext = originalCommandContext
		commandLookPath = originalLookPath
		repositoryInfoFunc = originalRepoInfoFunc
	}
}

func TestAddSubIssueSuccess(t *testing.T) {
	reset := stubGH(t)
	defer reset()

	t.Setenv("GH_HELPER_ISSUE_ID", "12345")
	t.Setenv("GH_HELPER_SUBISSUE_STATUS", "200")

	if err := AddSubIssue("5", "7"); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestAddSubIssueHTTPFailures(t *testing.T) {
	testCases := []struct {
		status string
		want   string
	}{
		{"404", "404"},
		{"422", "422"},
		{"401", "401"},
		{"403", "403"},
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			reset := stubGH(t)
			defer reset()

			t.Setenv("GH_HELPER_ISSUE_ID", "98765")
			t.Setenv("GH_HELPER_SUBISSUE_STATUS", tc.status)

			err := AddSubIssue("5", "7")
			if err == nil {
				t.Fatalf("expected error for status %s, got nil", tc.status)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}

	if sep == -1 {
		fmt.Fprintln(os.Stderr, "missing separator")
		os.Exit(1)
	}

	cmdArgs := args[sep+1:]
	if len(cmdArgs) == 0 || cmdArgs[0] != "gh" {
		fmt.Fprintf(os.Stderr, "unexpected command: %v", cmdArgs)
		os.Exit(1)
	}

	if len(cmdArgs) > 1 && cmdArgs[1] == "api" {
		joined := strings.Join(cmdArgs, " ")
		if strings.Contains(joined, "sub_issues") {
			status := os.Getenv("GH_HELPER_SUBISSUE_STATUS")
			switch status {
			case "404":
				fmt.Fprint(os.Stderr, "HTTP 404: Not Found")
				os.Exit(1)
			case "422":
				fmt.Fprint(os.Stderr, "HTTP 422: Unprocessable Entity")
				os.Exit(1)
			case "401":
				fmt.Fprint(os.Stderr, "HTTP 401: Unauthorized")
				os.Exit(1)
			case "403":
				fmt.Fprint(os.Stderr, "HTTP 403: Forbidden")
				os.Exit(1)
			default:
				os.Exit(0)
			}
		}

		if strings.Contains(joined, "--jq .id") {
			id := os.Getenv("GH_HELPER_ISSUE_ID")
			if id == "" {
				id = "123"
			}
			fmt.Fprint(os.Stdout, id)
			os.Exit(0)
		}
	}

	fmt.Fprintf(os.Stderr, "unhandled gh command: %v", cmdArgs)
	os.Exit(1)
}
