package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/model"
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
