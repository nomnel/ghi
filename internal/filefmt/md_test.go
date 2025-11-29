package filefmt

import (
	"testing"

	"github.com/nomnel/ghi/internal/model"
)

func TestEncodeMarkdownIncludesParent(t *testing.T) {
	parent := 42
	fm := model.Frontmatter{Title: "Child issue", Parent: &parent}
	body := []byte("Parent-aware body")

	got, err := EncodeMarkdown(fm, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "---\ntitle: Child issue\nparent: 42\n---\nParent-aware body"
	if string(got) != expected {
		t.Fatalf("frontmatter with parent mismatch\nexpected:\n%s\nactual:\n%s", expected, string(got))
	}
}

func TestEncodeMarkdownOmitsParentWhenUnset(t *testing.T) {
	fm := model.Frontmatter{Title: "Orphan issue"}
	body := []byte("Body without parent")

	got, err := EncodeMarkdown(fm, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "---\ntitle: Orphan issue\n---\nBody without parent"
	if string(got) != expected {
		t.Fatalf("frontmatter without parent mismatch\nexpected:\n%s\nactual:\n%s", expected, string(got))
	}
}
