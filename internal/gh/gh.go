package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nomnel/ghi/internal/model"
)

const commandTimeout = 30 * time.Second

func checkGHAvailable() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh CLI not found. Install GitHub CLI and run 'gh auth login'")
	}
	return nil
}

func ViewIssue(issueNumber string) (*model.IssueData, error) {
	if err := checkGHAvailable(); err != nil {
		return nil, err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "gh", "issue", "view", issueNumber, "--json", "title,body")
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return nil, fmt.Errorf("gh error: verify authentication ('gh auth status') and run inside a Git repo")
		}
		if strings.Contains(stderrStr, "not found") {
			return nil, fmt.Errorf("gh error: issue not found or repo not set. Authenticate with 'gh auth login' and run inside a repo")
		}
		return nil, fmt.Errorf("gh error: %s", stderrStr)
	}
	
	var issue model.IssueData
	if err := json.Unmarshal(stdout.Bytes(), &issue); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}
	
	return &issue, nil
}

func EditIssue(issueNumber string, title string, bodyFile string) error {
	if err := checkGHAvailable(); err != nil {
		return err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	args := []string{"issue", "edit", issueNumber}
	
	if title != "" && strings.TrimSpace(title) != "" {
		args = append(args, "--title", title)
	}
	
	args = append(args, "--body-file", bodyFile)
	
	cmd := exec.CommandContext(ctx, "gh", args...)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return fmt.Errorf("gh error: verify authentication ('gh auth status') and run inside a Git repo")
		}
		return fmt.Errorf("gh error: %s", stderrStr)
	}
	
	return nil
}

func CreateTempBodyFile(body []byte) (string, error) {
	tmp, err := os.CreateTemp("", "ghi-body-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write body to temp file: %w", err)
	}
	
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}
	
	return tmp.Name(), nil
}