package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/model"
	"github.com/spf13/cobra"
)

func TestUpdateParentFrontmatterAppendsAndKeepsOrder(t *testing.T) {
	dir := t.TempDir()
	issuePath := filepath.Join(dir, "12.md")

	content, err := filefmt.EncodeMarkdown(model.Frontmatter{Title: "Child title"}, []byte("body"))
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	if err := os.WriteFile(issuePath, content, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	if err := updateParentFrontmatter(issuePath, "7", 0o644); err != nil {
		t.Fatalf("update parent: %v", err)
	}

	raw, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("read updated: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	titleIdx, parentIdx, parentCount := -1, -1, 0
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "title:"):
			titleIdx = i
		case strings.HasPrefix(line, "parent:"):
			parentIdx = i
			parentCount++
		}
	}

	if parentCount != 1 {
		t.Fatalf("expected one parent entry, got %d", parentCount)
	}
	if !(parentIdx > titleIdx && parentIdx != -1) {
		t.Fatalf("expected parent after title, got title=%d parent=%d", titleIdx, parentIdx)
	}

	fm, _, err := filefmt.DecodeMarkdown(raw)
	if err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if fm.Parent != "7" {
		t.Fatalf("expected parent=7, got %s", fm.Parent)
	}
}

func TestUpdateParentFrontmatterPreservesUnknownKeysAndAppendsParent(t *testing.T) {
	dir := t.TempDir()
	issuePath := filepath.Join(dir, "12.md")

	seed := "---\ntitle: Child title\ncustom: foo\nlabels:\n  - a\n  - b\n---\nbody"
	if err := os.WriteFile(issuePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	if err := updateParentFrontmatter(issuePath, "7", 0o644); err != nil {
		t.Fatalf("update parent: %v", err)
	}

	raw, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("read updated: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	indices := map[string]int{}
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "title:"):
			indices["title"] = i
		case strings.HasPrefix(line, "custom:"):
			indices["custom"] = i
		case strings.HasPrefix(line, "labels:"):
			indices["labels"] = i
		case strings.HasPrefix(line, "parent:"):
			indices["parent"] = i
		}
	}

	required := []string{"title", "custom", "labels", "parent"}
	for _, key := range required {
		if _, ok := indices[key]; !ok {
			t.Fatalf("expected %s to be present", key)
		}
	}

	if !(indices["title"] < indices["custom"] && indices["custom"] < indices["labels"] && indices["labels"] < indices["parent"]) {
		t.Fatalf("key order changed: %#v", indices)
	}
}

func TestEnsureParentFrontmatterDetectsExistingParent(t *testing.T) {
	dir := t.TempDir()
	issuePath := filepath.Join(dir, "5.md")

	content, err := filefmt.EncodeMarkdown(model.Frontmatter{Title: "Existing", Parent: "3"}, []byte("body text"))
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	if err := os.WriteFile(issuePath, content, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	already, err := ensureParentFrontmatter(issuePath, "3")
	if err != nil {
		t.Fatalf("ensure parent: %v", err)
	}
	if !already {
		t.Fatalf("expected to detect existing parent")
	}
}

func TestRunLinkValidationErrors(t *testing.T) {
	origAdd := addSubIssue
	defer func() { addSubIssue = origAdd }()

	tests := []struct {
		name     string
		args     []string
		parent   string
		wantCode model.ErrorType
	}{
		{"sameNumbers", []string{"5"}, "5", model.ExitUsage},
		{"nonNumericChild", []string{"abc"}, "9", model.ExitUsage},
		{"nonNumericParent", []string{"10"}, "p9", model.ExitUsage},
	}

	addSubIssue = func(childNumber, parentNumber string) error {
		t.Fatalf("addSubIssue should not be called for validation errors")
		return nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linkParent = tt.parent
			err := runLink(&cobra.Command{}, tt.args)
			if err == nil {
				t.Fatalf("expected error")
			}
			exitErr, ok := err.(*model.ExitError)
			if !ok {
				t.Fatalf("expected ExitError, got %T", err)
			}
			if exitErr.Code != tt.wantCode {
				t.Fatalf("unexpected code: %v", exitErr.Code)
			}
		})
	}
}

func TestRunLinkRemoteErrorKeepsFileUntouched(t *testing.T) {
	wd := mustChdirTemp(t)
	_ = wd
	if err := os.MkdirAll("issues", 0o755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	origAdd := addSubIssue
	defer func() { addSubIssue = origAdd }()

	issuePath := filepath.Join("issues", "4.md")
	initial, err := filefmt.EncodeMarkdown(model.Frontmatter{Title: "Child"}, []byte("body"))
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	if err := os.WriteFile(issuePath, initial, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	linkParent = "9"
	addSubIssue = func(childNumber, parentNumber string) error {
		return fmt.Errorf("remote failure")
	}

	err = runLink(&cobra.Command{}, []string{"4"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if exitErr, ok := err.(*model.ExitError); !ok || exitErr.Code != model.ExitEnv {
		t.Fatalf("expected ExitEnv, got %v", err)
	}

	updated, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(updated) != string(initial) {
		t.Fatalf("file mutated on remote error")
	}
}

func TestRunLinkCallsRemoteWhenParentAlreadySet(t *testing.T) {
	wd := mustChdirTemp(t)
	_ = wd
	if err := os.MkdirAll("issues", 0o755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	origAdd := addSubIssue
	defer func() { addSubIssue = origAdd }()

	issuePath := filepath.Join("issues", "6.md")
	seed := "---\ntitle: Existing\nparent: 2\ncustom: keep\n---\nbody"
	if err := os.WriteFile(issuePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	linkParent = "2"
	called := 0
	addSubIssue = func(childNumber, parentNumber string) error {
		called++
		return nil
	}

	if err := runLink(&cobra.Command{}, []string{"6"}); err != nil {
		t.Fatalf("runLink: %v", err)
	}

	if called != 1 {
		t.Fatalf("expected remote call, got %d", called)
	}

	updated, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	parentCount := 0
	for _, line := range strings.Split(string(updated), "\n") {
		if strings.HasPrefix(line, "parent:") {
			parentCount++
		}
	}
	if parentCount != 1 {
		t.Fatalf("expected single parent entry, got %d", parentCount)
	}
}

func mustChdirTemp(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	newWD := t.TempDir()
	if err := os.Chdir(newWD); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return newWD
}
