package filefmt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nomnel/ghi/internal/model"
	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

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

func DecodeMarkdown(raw []byte) (*model.Frontmatter, []byte, error) {
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
	
	var fm model.Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterContent), &fm); err != nil {
		return nil, nil, fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}
	
	bodyStartIdx := closingIdx + 1
	var bodyLines []string
	if bodyStartIdx < len(lines) {
		bodyLines = lines[bodyStartIdx:]
	}
	body := []byte(strings.Join(bodyLines, "\n"))
	
	return &fm, body, nil
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