package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/gh"
	"github.com/nomnel/ghi/internal/model"
	"github.com/spf13/cobra"
)

func prepareLinkFixture(t *testing.T, parent *int) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	if err := os.MkdirAll("issues", 0o755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	fm := model.Frontmatter{Title: "Child title", Parent: parent}
	body := []byte("child body")
	content, err := filefmt.EncodeMarkdown(fm, body)
	if err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}

	path := filepath.Join("issues", "5.md")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	return path
}

func TestRunLinkAddsParentAndUpdatesFile(t *testing.T) {
	resetDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to capture cwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(resetDir) })

	path := prepareLinkFixture(t, nil)

	called := false
	addSubIssueFn = func(child, parent string) error {
		called = true
		if child != "5" || parent != "7" {
			t.Fatalf("unexpected parameters child=%s parent=%s", child, parent)
		}
		return nil
	}
	t.Cleanup(func() { addSubIssueFn = gh.AddSubIssue })

	cmd := &cobra.Command{}
	cmd.Flags().String("parent", "", "")
	cmd.Flags().Set("parent", "7")

	if err := runLink(cmd, []string{"5"}); err != nil {
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
		t.Fatalf("frontmatter order or content mismatch\nexpected:\n%s\nactual:\n%s", expected, string(updated))
	}
}

func TestRunLinkSkipsWhenAlreadyLinked(t *testing.T) {
	resetDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to capture cwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(resetDir) })

	parent := 7
	path := prepareLinkFixture(t, &parent)

	addSubIssueFn = func(_, _ string) error {
		t.Fatalf("AddSubIssue should not be called when parent matches")
		return nil
	}
	t.Cleanup(func() { addSubIssueFn = gh.AddSubIssue })

	cmd := &cobra.Command{}
	cmd.Flags().String("parent", "", "")
	cmd.Flags().Set("parent", "7")

	if err := runLink(cmd, []string{"5"}); err != nil {
		t.Fatalf("runLink returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "---\ntitle: Child title\nparent: 7\n---\nchild body"
	if string(data) != expected {
		t.Fatalf("file should remain unchanged\nexpected:\n%s\nactual:\n%s", expected, string(data))
	}
}

func TestRunLinkRejectsSameNumbers(t *testing.T) {
	resetDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to capture cwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(resetDir) })

	prepareLinkFixture(t, nil)

	cmd := &cobra.Command{}
	cmd.Flags().String("parent", "", "")
	cmd.Flags().Set("parent", "5")

	err = runLink(cmd, []string{"5"})
	if err == nil {
		t.Fatalf("expected usage error when child equals parent")
	}

	exitErr, ok := err.(*model.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != model.ExitUsage {
		t.Fatalf("expected usage exit code, got %v", exitErr.Code)
	}
}
