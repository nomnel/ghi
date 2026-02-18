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

func TestFrontmatterDocBlockedByNormalizesValues(t *testing.T) {
	raw := []byte("---\ntitle: Child issue\nblocked_by:\n  - 5\n  - 3\n  - 5\n---\nbody")

	fm, _, err := DecodeMarkdown(raw)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	got, err := fm.BlockedBy()
	if err != nil {
		t.Fatalf("unexpected blocked_by parse error: %v", err)
	}

	want := []int{3, 5}
	if len(got) != len(want) {
		t.Fatalf("unexpected blocked_by length: got=%v want=%v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected blocked_by values: got=%v want=%v", got, want)
		}
	}
}

func TestFrontmatterDocSetBlockedByPreservesUnknownKeyOrder(t *testing.T) {
	raw := []byte("---\ntitle: Child issue\nlabels:\n  - bug\nblocked_by: [8, 4]\ncustom: keep\n---\nbody")

	fm, body, err := DecodeMarkdown(raw)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	fm.SetBlockedBy([]int{7, 4, 7, 1})

	content, err := EncodeFrontmatterDoc(fm, body)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}

	expected := "---\ntitle: Child issue\nlabels:\n  - bug\ncustom: keep\nblocked_by: [1, 4, 7]\n---\nbody"
	if string(content) != expected {
		t.Fatalf("frontmatter order/content mismatch\nexpected:\n%s\nactual:\n%s", expected, string(content))
	}
}

func TestFrontmatterDocBlockedByRejectsInvalidValues(t *testing.T) {
	raw := []byte("---\ntitle: Child issue\nblocked_by: [3, nope]\n---\nbody")

	fm, _, err := DecodeMarkdown(raw)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if _, err := fm.BlockedBy(); err == nil {
		t.Fatalf("expected blocked_by parse error")
	}
}
