package filefmt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nomnel/ghi/internal/model"
	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

// FrontmatterDoc preserves the original mapping order and unknown keys while
// exposing helpers to read and mutate known fields.
type FrontmatterDoc struct {
	mapping *yaml.Node
}

// Title returns the title value if present.
func (fm *FrontmatterDoc) Title() string {
	for i := 0; i+1 < len(fm.mapping.Content); i += 2 {
		if fm.mapping.Content[i].Value == "title" {
			return fm.mapping.Content[i+1].Value
		}
	}
	return ""
}

// Parent returns the parent issue number if present and valid.
func (fm *FrontmatterDoc) Parent() (*int, error) {
	for i := 0; i+1 < len(fm.mapping.Content); i += 2 {
		if fm.mapping.Content[i].Value != "parent" {
			continue
		}

		v := strings.TrimSpace(fm.mapping.Content[i+1].Value)
		if v == "" {
			return nil, nil
		}

		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid parent value: %w", err)
		}
		return &n, nil
	}

	return nil, nil
}

// HasParent reports whether parent matches the stored parent value.
func (fm *FrontmatterDoc) HasParent(parent int) bool {
	current, err := fm.Parent()
	if err != nil || current == nil {
		return false
	}
	return *current == parent
}

// SetParent removes any existing parent entry and appends a new one at the end
// while leaving other keys and their order intact.
func (fm *FrontmatterDoc) SetParent(parent int) {
	cleaned := make([]*yaml.Node, 0, len(fm.mapping.Content))
	for i := 0; i+1 < len(fm.mapping.Content); i += 2 {
		if fm.mapping.Content[i].Value == "parent" {
			continue
		}
		cleaned = append(cleaned, fm.mapping.Content[i], fm.mapping.Content[i+1])
	}

	fm.mapping.Content = cleaned

	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "parent"}
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(parent)}
	fm.mapping.Content = append(fm.mapping.Content, key, val)
}

func EncodeMarkdown(fm model.Frontmatter, body []byte) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(frontmatterDelimiter + "\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return nil, fmt.Errorf("failed to encode frontmatter: %w", err)
	}
	encoder.Close()

	buf.WriteString(frontmatterDelimiter + "\n")

	buf.Write(body)

	return buf.Bytes(), nil
}

func DecodeMarkdown(raw []byte) (*FrontmatterDoc, []byte, error) {
	content := string(raw)
	lines := strings.Split(content, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelimiter {
		return nil, nil, fmt.Errorf("%w: file must start with '---'", model.ErrMalformedFrontmatter)
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelimiter {
			closingIdx = i
			break
		}
	}

	if closingIdx == -1 {
		return nil, nil, fmt.Errorf("%w: missing closing '---'", model.ErrMalformedFrontmatter)
	}

	frontmatterContent := strings.Join(lines[1:closingIdx], "\n")

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatterContent), &doc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%w: frontmatter must be a mapping", model.ErrMalformedFrontmatter)
	}

	bodyStartIdx := closingIdx + 1
	var bodyLines []string
	if bodyStartIdx < len(lines) {
		bodyLines = lines[bodyStartIdx:]
	}
	body := []byte(strings.Join(bodyLines, "\n"))

	return &FrontmatterDoc{mapping: doc.Content[0]}, body, nil
}

func EncodeFrontmatterDoc(fm *FrontmatterDoc, body []byte) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(frontmatterDelimiter + "\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm.mapping); err != nil {
		return nil, fmt.Errorf("failed to encode frontmatter: %w", err)
	}
	encoder.Close()

	buf.WriteString(frontmatterDelimiter + "\n")

	buf.Write(body)

	return buf.Bytes(), nil
}

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, fmt.Sprintf(".%s-*.tmp", filepath.Base(path)))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmp = nil

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
