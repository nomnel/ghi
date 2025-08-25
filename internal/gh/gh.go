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

func checkGitAvailable() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found")
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

func RunGitDiff(localPath, remotePath string, extraArgs []string) (int, error) {
	if err := checkGitAvailable(); err != nil {
		return 2, err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	args := []string{"--no-pager", "diff", "--no-index", "--exit-code"}
	args = append(args, extraArgs...)
	args = append(args, "--", localPath, remotePath)
	
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 2, fmt.Errorf("git diff failed: %w", err)
	}
	
	return 0, nil
}

func GetRepositoryInfo() (owner string, repo string, err error) {
	if err := checkGHAvailable(); err != nil {
		return "", "", err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return "", "", fmt.Errorf("gh CLI error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		if strings.Contains(stderrStr, "not a git repository") || strings.Contains(stderrStr, "not found") {
			return "", "", fmt.Errorf("gh CLI error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		return "", "", fmt.Errorf("gh error: %s", stderrStr)
	}
	
	nameWithOwner := strings.TrimSpace(stdout.String())
	parts := strings.Split(nameWithOwner, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected repository format: %s", nameWithOwner)
	}
	
	return parts[0], parts[1], nil
}

type CreateIssueResponse struct {
	Number int `json:"number"`
}

func CreateIssue(title string) (int, error) {
	if err := checkGHAvailable(); err != nil {
		return 0, err
	}
	
	owner, repo, err := GetRepositoryInfo()
	if err != nil {
		return 0, err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	apiPath := fmt.Sprintf("repos/%s/%s/issues", owner, repo)
	cmd := exec.CommandContext(ctx, "gh", "api", "--method", "POST",
		"-H", "Accept: application/vnd.github+json",
		apiPath,
		"-f", "title="+title)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return 0, fmt.Errorf("gh error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		return 0, fmt.Errorf("gh api error: %s", stderrStr)
	}
	
	var response CreateIssueResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return 0, fmt.Errorf("failed to parse API response: %w", err)
	}
	
	if response.Number == 0 {
		return 0, fmt.Errorf("API response missing issue number")
	}
	
	return response.Number, nil
}

func CloseIssue(issueNumber string) error {
	if err := checkGHAvailable(); err != nil {
		return err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "gh", "issue", "close", issueNumber)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return fmt.Errorf("gh error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		if strings.Contains(stderrStr, "not found") || strings.Contains(stderrStr, "404") {
			return fmt.Errorf("gh error: issue not found or repo not set")
		}
		if strings.Contains(stderrStr, "permission") || strings.Contains(stderrStr, "forbidden") {
			return fmt.Errorf("gh error: permission denied")
		}
		return fmt.Errorf("gh error: %s", stderrStr)
	}
	
	// Check if gh printed output - if not, we'll print our own success message
	if stdoutStr := strings.TrimSpace(stdout.String()); stdoutStr != "" {
		fmt.Print(stdoutStr)
		if !strings.HasSuffix(stdoutStr, "\n") {
			fmt.Println()
		}
	} else {
		fmt.Printf("Closed issue #%s.\n", issueNumber)
	}
	
	return nil
}

func ReopenIssue(issueNumber string) error {
	if err := checkGHAvailable(); err != nil {
		return err
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "gh", "issue", "reopen", issueNumber)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return fmt.Errorf("gh error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		if strings.Contains(stderrStr, "not found") || strings.Contains(stderrStr, "404") {
			return fmt.Errorf("gh error: issue not found or repo not set")
		}
		if strings.Contains(stderrStr, "permission") || strings.Contains(stderrStr, "forbidden") {
			return fmt.Errorf("gh error: permission denied")
		}
		return fmt.Errorf("gh error: %s", stderrStr)
	}
	
	// Check if gh printed output - if not, we'll print our own success message
	if stdoutStr := strings.TrimSpace(stdout.String()); stdoutStr != "" {
		fmt.Print(stdoutStr)
		if !strings.HasSuffix(stdoutStr, "\n") {
			fmt.Println()
		}
	} else {
		fmt.Printf("Reopened issue #%s.\n", issueNumber)
	}
	
	return nil
}