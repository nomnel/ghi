package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/model"
	"github.com/spf13/cobra"
)

func setupLinkWorkspace(t *testing.T) {
	t.Helper()

	resetDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to capture cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(resetDir) })

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	if err := os.MkdirAll("issues", 0o755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}
}

func writeIssueFixture(t *testing.T, issueNumber int, fm model.Frontmatter, body string) string {
	t.Helper()

	content, err := filefmt.EncodeMarkdown(fm, []byte(body))
	if err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}

	path := filepath.Join("issues", fmt.Sprintf("%d.md", issueNumber))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	return path
}

func writeIssueRaw(t *testing.T, issueNumber int, content string) string {
	t.Helper()

	path := filepath.Join("issues", fmt.Sprintf("%d.md", issueNumber))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write issue markdown: %v", err)
	}

	return path
}

func newLinkTestCommand(t *testing.T, parent string, blockedBy []string, dependsOn []string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String("parent", "", "")
	cmd.Flags().StringArray("blocked-by", nil, "")
	cmd.Flags().StringArray("depends-on", nil, "")

	if parent != "" {
		if err := cmd.Flags().Set("parent", parent); err != nil {
			t.Fatalf("failed to set parent flag: %v", err)
		}
	}
	for _, blocker := range blockedBy {
		if err := cmd.Flags().Set("blocked-by", blocker); err != nil {
			t.Fatalf("failed to set blocked-by flag: %v", err)
		}
	}
	for _, dep := range dependsOn {
		if err := cmd.Flags().Set("depends-on", dep); err != nil {
			t.Fatalf("failed to set depends-on flag: %v", err)
		}
	}

	return cmd
}

func stubLinkFns(t *testing.T, subIssue func(string, string) error, blockedBy func(string, string) error) {
	t.Helper()

	origSub := addSubIssueFn
	origBlocked := addBlockedByFn

	addSubIssueFn = subIssue
	addBlockedByFn = blockedBy

	t.Cleanup(func() {
		addSubIssueFn = origSub
		addBlockedByFn = origBlocked
	})
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	_ = r.Close()

	return string(out), runErr
}

func expectExitCode(t *testing.T, err error, code model.ErrorType) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	exitErr, ok := err.(*model.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != code {
		t.Fatalf("unexpected exit code: got=%v want=%v", exitErr.Code, code)
	}
}

func TestRunLinkParentOnly(t *testing.T) {
	setupLinkWorkspace(t)

	path := writeIssueFixture(t, 5, model.Frontmatter{Title: "Child title"}, "child body")

	called := false
	stubLinkFns(t,
		func(child, parent string) error {
			called = true
			if child != "5" || parent != "7" {
				t.Fatalf("unexpected sub-issue params: child=%s parent=%s", child, parent)
			}
			return nil
		},
		func(_, _ string) error {
			t.Fatalf("AddBlockedBy should not be called")
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "7", nil, nil)
	if _, err := captureStdout(t, func() error { return runLink(cmd, []string{"5"}) }); err != nil {
		t.Fatalf("runLink returned error: %v", err)
	}

	if !called {
		t.Fatalf("expected AddSubIssue to be called")
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	expected := "---\ntitle: Child title\nparent: 7\n---\nchild body"
	if string(updated) != expected {
		t.Fatalf("unexpected content\nexpected:\n%s\nactual:\n%s", expected, string(updated))
	}
}

func TestRunLinkBlockedByOnlyUsesURLAndMergesBlockedBy(t *testing.T) {
	setupLinkWorkspace(t)

	initial := "---\ntitle: Child title\ncustom: keep\nblocked_by: [10, 3]\n---\nchild body"
	path := writeIssueRaw(t, 5, initial)

	var calls []string
	stubLinkFns(t,
		func(_, _ string) error {
			t.Fatalf("AddSubIssue should not be called")
			return nil
		},
		func(issue, blocker string) error {
			calls = append(calls, issue+"<-"+blocker)
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "", []string{
		"https://github.com/org/repo/issues/8",
		"8",
		"3",
	}, nil)

	out, err := captureStdout(t, func() error {
		return runLink(cmd, []string{"https://github.com/org/repo/issues/5"})
	})
	if err != nil {
		t.Fatalf("runLink returned error: %v", err)
	}

	if len(calls) != 1 || calls[0] != "5<-8" {
		t.Fatalf("unexpected blocked-by calls: %v", calls)
	}

	expectedSummary := "link summary: parent=not-requested blocked_by_added=1/2 partial_success=no\n"
	if out != expectedSummary {
		t.Fatalf("unexpected summary\nexpected:\n%s\nactual:\n%s", expectedSummary, out)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	expected := "---\ntitle: Child title\ncustom: keep\nblocked_by: [3, 8, 10]\n---\nchild body"
	if string(updated) != expected {
		t.Fatalf("unexpected content\nexpected:\n%s\nactual:\n%s", expected, string(updated))
	}
}

func TestRunLinkParentAndBlockedBySuccess(t *testing.T) {
	setupLinkWorkspace(t)

	path := writeIssueFixture(t, 5, model.Frontmatter{Title: "Child title"}, "child body")

	subCalled := false
	blockedCalled := false
	stubLinkFns(t,
		func(child, parent string) error {
			subCalled = true
			if child != "5" || parent != "7" {
				t.Fatalf("unexpected sub-issue params: child=%s parent=%s", child, parent)
			}
			return nil
		},
		func(issue, blocker string) error {
			blockedCalled = true
			if issue != "5" || blocker != "9" {
				t.Fatalf("unexpected blocked-by params: issue=%s blocker=%s", issue, blocker)
			}
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "7", []string{"9"}, nil)
	out, err := captureStdout(t, func() error { return runLink(cmd, []string{"5"}) })
	if err != nil {
		t.Fatalf("runLink returned error: %v", err)
	}

	if !subCalled || !blockedCalled {
		t.Fatalf("expected both AddSubIssue and AddBlockedBy to be called")
	}

	expectedSummary := "link summary: parent=added blocked_by_added=1/1 partial_success=no\n"
	if out != expectedSummary {
		t.Fatalf("unexpected summary\nexpected:\n%s\nactual:\n%s", expectedSummary, out)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	expected := "---\ntitle: Child title\nparent: 7\nblocked_by: [9]\n---\nchild body"
	if string(updated) != expected {
		t.Fatalf("unexpected content\nexpected:\n%s\nactual:\n%s", expected, string(updated))
	}
}

func TestRunLinkRequiresAtLeastOneLinkOption(t *testing.T) {
	setupLinkWorkspace(t)
	writeIssueFixture(t, 5, model.Frontmatter{Title: "Child title"}, "child body")

	stubLinkFns(t,
		func(_, _ string) error {
			t.Fatalf("AddSubIssue should not be called")
			return nil
		},
		func(_, _ string) error {
			t.Fatalf("AddBlockedBy should not be called")
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "", nil, nil)
	err := runLink(cmd, []string{"5"})
	expectExitCode(t, err, model.ExitUsage)
}

func TestRunLinkRejectsDependsOnFlag(t *testing.T) {
	setupLinkWorkspace(t)
	writeIssueFixture(t, 5, model.Frontmatter{Title: "Child title"}, "child body")

	cmd := newLinkTestCommand(t, "", nil, []string{"8"})
	err := runLink(cmd, []string{"5"})
	expectExitCode(t, err, model.ExitUsage)

	if !strings.Contains(err.Error(), "--depends-on is not supported") {
		t.Fatalf("expected unsupported --depends-on message, got: %v", err)
	}
}

func TestRunLinkValidationsSkipAPI(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		parent    string
		blockedBy []string
	}{
		{name: "issue equals parent", args: []string{"5"}, parent: "5"},
		{name: "blocked-by contains issue", args: []string{"5"}, blockedBy: []string{"5"}},
		{name: "blocked-by contains parent", args: []string{"5"}, parent: "7", blockedBy: []string{"7"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupLinkWorkspace(t)
			writeIssueFixture(t, 5, model.Frontmatter{Title: "Child title"}, "child body")

			subCalls := 0
			blockedCalls := 0
			stubLinkFns(t,
				func(_, _ string) error {
					subCalls++
					return nil
				},
				func(_, _ string) error {
					blockedCalls++
					return nil
				},
			)

			cmd := newLinkTestCommand(t, tc.parent, tc.blockedBy, nil)
			err := runLink(cmd, tc.args)
			expectExitCode(t, err, model.ExitUsage)

			if subCalls != 0 || blockedCalls != 0 {
				t.Fatalf("API should not be called on validation failure: sub=%d blocked=%d", subCalls, blockedCalls)
			}
		})
	}
}

func TestRunLinkKeepsBlockedByWhenFlagIsNotProvided(t *testing.T) {
	setupLinkWorkspace(t)

	initial := "---\ntitle: Child title\nblocked_by:\n  - 4\n  - 2\n---\nchild body"
	path := writeIssueRaw(t, 5, initial)

	stubLinkFns(t,
		func(child, parent string) error {
			if child != "5" || parent != "7" {
				t.Fatalf("unexpected sub-issue params: child=%s parent=%s", child, parent)
			}
			return nil
		},
		func(_, _ string) error {
			t.Fatalf("AddBlockedBy should not be called")
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "7", nil, nil)
	if _, err := captureStdout(t, func() error { return runLink(cmd, []string{"5"}) }); err != nil {
		t.Fatalf("runLink returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	expected := "---\ntitle: Child title\nblocked_by:\n  - 4\n  - 2\nparent: 7\n---\nchild body"
	if string(updated) != expected {
		t.Fatalf("blocked_by should remain unchanged when flag is omitted\nexpected:\n%s\nactual:\n%s", expected, string(updated))
	}
}

func TestRunLinkLeavesFileUntouchedWhenRemoteFailsAfterPartialSuccess(t *testing.T) {
	setupLinkWorkspace(t)

	initial := "---\ntitle: Child title\nextra: keep\n---\nbody"
	path := writeIssueRaw(t, 5, initial)

	stubLinkFns(t,
		func(child, parent string) error {
			if child != "5" || parent != "7" {
				t.Fatalf("unexpected sub-issue params: child=%s parent=%s", child, parent)
			}
			return nil
		},
		func(_, _ string) error {
			return errors.New("blocked-by failure")
		},
	)

	cmd := newLinkTestCommand(t, "7", []string{"9"}, nil)
	out, err := captureStdout(t, func() error { return runLink(cmd, []string{"5"}) })
	expectExitCode(t, err, model.ExitEnv)

	expectedSummary := "link summary: parent=added blocked_by_added=0/1 partial_success=yes\n"
	if out != expectedSummary {
		t.Fatalf("unexpected summary\nexpected:\n%s\nactual:\n%s", expectedSummary, out)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read file: %v", readErr)
	}
	if string(data) != initial {
		t.Fatalf("file should remain unchanged after remote failure\nexpected:\n%s\nactual:\n%s", initial, string(data))
	}
}

func TestRunLinkLeavesFileUntouchedWhenParentLinkFails(t *testing.T) {
	setupLinkWorkspace(t)

	initial := "---\ntitle: Child title\nextra: keep\n---\nbody"
	path := writeIssueRaw(t, 5, initial)

	stubLinkFns(t,
		func(_, _ string) error {
			return errors.New("parent failure")
		},
		func(_, _ string) error {
			t.Fatalf("AddBlockedBy should not be called when parent link fails first")
			return nil
		},
	)

	cmd := newLinkTestCommand(t, "7", []string{"9"}, nil)
	out, err := captureStdout(t, func() error { return runLink(cmd, []string{"5"}) })
	expectExitCode(t, err, model.ExitEnv)

	expectedSummary := "link summary: parent=failed blocked_by_added=0/1 partial_success=no\n"
	if out != expectedSummary {
		t.Fatalf("unexpected summary\nexpected:\n%s\nactual:\n%s", expectedSummary, out)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read file: %v", readErr)
	}
	if string(data) != initial {
		t.Fatalf("file should remain unchanged after remote failure\nexpected:\n%s\nactual:\n%s", initial, string(data))
	}
}
