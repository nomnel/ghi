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

var (
	commandContext     = exec.CommandContext
	commandLookPath    = exec.LookPath
	repositoryInfoFunc = realGetRepositoryInfo
)

func checkGHAvailable() error {
	_, err := commandLookPath("gh")
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

	owner, repo, err := GetRepositoryInfo()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	query := `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      title
      body
      parent { number }
    }
  }
}`

	cmd := commandContext(ctx, "gh", "api", "graphql",
		"-H", "GraphQL-Features: sub_issues",
		"-f", "query="+query,
		"-f", "owner="+owner,
		"-f", "name="+repo,
		"-F", "number="+issueNumber,
	)

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

	var response struct {
		Data struct {
			Repository struct {
				Issue *model.IssueData `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	if len(response.Errors) > 0 && response.Data.Repository.Issue == nil {
		var messages []string
		for _, e := range response.Errors {
			if e.Message != "" {
				messages = append(messages, e.Message)
			}
		}
		errMsg := strings.Join(messages, "; ")
		if strings.Contains(errMsg, "Could not resolve to an Issue") {
			return nil, fmt.Errorf("gh error: issue not found or repo not set. Authenticate with 'gh auth login' and run inside a repo")
		}
		return nil, fmt.Errorf("gh error: %s", errMsg)
	}

	issue := response.Data.Repository.Issue
	if issue == nil {
		return nil, fmt.Errorf("gh error: issue not found or repo not set. Authenticate with 'gh auth login' and run inside a repo")
	}

	if len(response.Errors) > 0 {
		var messages []string
		for _, e := range response.Errors {
			if e.Message != "" {
				messages = append(messages, e.Message)
			}
		}
		if len(messages) > 0 {
			fmt.Fprintf(os.Stderr, "warning: partial GraphQL response: %s\n", strings.Join(messages, "; "))
		}
	}

	return issue, nil
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

	cmd := commandContext(ctx, "gh", args...)

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

	cmd := commandContext(ctx, "git", args...)
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

type linkMutationType string
type graphQLError struct {
	Message string `json:"message"`
}

const (
	linkMutationSubIssue  linkMutationType = "sub_issue"
	linkMutationBlockedBy linkMutationType = "blocked_by"
)

func GetIssueID(issueNumber string) (string, error) {
	if err := checkGHAvailable(); err != nil {
		return "", err
	}

	owner, repo, err := GetRepositoryInfo()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	query := `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      id
    }
  }
}`

	cmd := commandContext(ctx, "gh", "api", "graphql",
		"-f", "query="+query,
		"-f", "owner="+owner,
		"-f", "name="+repo,
		"-F", "number="+issueNumber,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.Contains(stderrStr, "404") || strings.Contains(strings.ToLower(stderrStr), "not found") {
			return "", fmt.Errorf("gh error: issue #%s not found", issueNumber)
		}
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "401") {
			return "", fmt.Errorf("gh error: authentication required")
		}
		return "", fmt.Errorf("gh error: %s", stderrStr)
	}

	var response struct {
		Data struct {
			Repository struct {
				Issue *struct {
					ID string `json:"id"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return "", fmt.Errorf("failed to parse gh output: %w", err)
	}

	if len(response.Errors) > 0 && response.Data.Repository.Issue == nil {
		errorText := collectGraphQLErrorMessages(response.Errors)
		if strings.Contains(errorText, "Could not resolve to an Issue") {
			return "", fmt.Errorf("gh error: issue #%s not found", issueNumber)
		}
		return "", fmt.Errorf("gh error: %s", errorText)
	}

	if response.Data.Repository.Issue == nil || strings.TrimSpace(response.Data.Repository.Issue.ID) == "" {
		return "", fmt.Errorf("gh error: issue id missing for #%s", issueNumber)
	}

	return response.Data.Repository.Issue.ID, nil
}

func AddSubIssue(childNumber, parentNumber string) error {
	if err := checkGHAvailable(); err != nil {
		return err
	}

	if childNumber == parentNumber {
		return fmt.Errorf("child and parent issue cannot be the same")
	}

	childID, err := GetIssueID(childNumber)
	if err != nil {
		return err
	}

	parentID, err := GetIssueID(parentNumber)
	if err != nil {
		return err
	}

	mutation := `mutation($issueId: ID!, $subIssueId: ID!) {
  addSubIssue(input: {issueId: $issueId, subIssueId: $subIssueId, replaceParent: true}) {
    clientMutationId
  }
}`

	return runIssueLinkMutation(
		linkMutationSubIssue,
		mutation,
		"issueId", parentID,
		"subIssueId", childID,
	)
}

func AddBlockedBy(issueNumber, blockingIssueNumber string) error {
	if err := checkGHAvailable(); err != nil {
		return err
	}

	if issueNumber == blockingIssueNumber {
		return fmt.Errorf("issue and blocking issue cannot be the same")
	}

	issueID, err := GetIssueID(issueNumber)
	if err != nil {
		return err
	}

	blockingIssueID, err := GetIssueID(blockingIssueNumber)
	if err != nil {
		return err
	}

	mutation := `mutation($issueId: ID!, $blockingIssueId: ID!) {
  addBlockedBy(input: {issueId: $issueId, blockingIssueId: $blockingIssueId}) {
    clientMutationId
  }
}`

	return runIssueLinkMutation(
		linkMutationBlockedBy,
		mutation,
		"issueId", issueID,
		"blockingIssueId", blockingIssueID,
	)
}

func runIssueLinkMutation(mutationType linkMutationType, mutation string, variablePairs ...string) error {
	if len(variablePairs)%2 != 0 {
		return fmt.Errorf("internal error: variable pairs must be key/value")
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	args := []string{"api", "graphql", "-H", "GraphQL-Features: sub_issues", "-f", "query=" + mutation}
	for i := 0; i < len(variablePairs); i += 2 {
		key := variablePairs[i]
		value := variablePairs[i+1]
		args = append(args, "-f", key+"="+value)
	}

	cmd := commandContext(ctx, "gh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return mapIssueLinkError(mutationType, strings.TrimSpace(stderr.String()))
	}

	var response struct {
		Errors []graphQLError `json:"errors"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return fmt.Errorf("failed to parse gh output: %w", err)
	}

	if len(response.Errors) > 0 {
		return mapIssueLinkError(mutationType, collectGraphQLErrorMessages(response.Errors))
	}

	return nil
}

func collectGraphQLErrorMessages(errors []graphQLError) string {
	messages := make([]string, 0, len(errors))
	for _, entry := range errors {
		if strings.TrimSpace(entry.Message) != "" {
			messages = append(messages, strings.TrimSpace(entry.Message))
		}
	}

	return strings.Join(messages, "; ")
}

func mapIssueLinkError(mutationType linkMutationType, raw string) error {
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(raw, "401") || strings.Contains(lower, "authentication") || strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("gh error: authentication required (HTTP 401)")
	case strings.Contains(raw, "403") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "resource not accessible"):
		if mutationType == linkMutationSubIssue {
			return fmt.Errorf("gh error: forbidden: ensure sub-issues are enabled and you have permissions (HTTP 403)")
		}
		return fmt.Errorf("gh error: forbidden: ensure issue dependencies are enabled and you have permissions (HTTP 403)")
	case strings.Contains(raw, "404") || strings.Contains(lower, "not found") || strings.Contains(lower, "could not resolve"):
		if mutationType == linkMutationSubIssue {
			return fmt.Errorf("gh error: parent issue not found or sub-issues disabled (HTTP 404)")
		}
		return fmt.Errorf("gh error: dependency issue not found (HTTP 404)")
	case strings.Contains(raw, "422") || strings.Contains(lower, "unprocessable"):
		if mutationType == linkMutationSubIssue {
			return fmt.Errorf("gh error: sub-issue request rejected (HTTP 422): %s", raw)
		}
		return fmt.Errorf("gh error: dependency request rejected (HTTP 422): %s", raw)
	default:
		return fmt.Errorf("gh error: %s", raw)
	}
}

func GetRepositoryInfo() (owner string, repo string, err error) {
	return repositoryInfoFunc()
}

func realGetRepositoryInfo() (owner string, repo string, err error) {
	if err := checkGHAvailable(); err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := commandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")

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
	cmd := commandContext(ctx, "gh", "api", "--method", "POST",
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

	cmd := commandContext(ctx, "gh", "issue", "close", issueNumber)

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

	cmd := commandContext(ctx, "gh", "issue", "reopen", issueNumber)

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

func ListIssues(extraArgs []string) ([]model.IssueListItem, error) {
	if err := checkGHAvailable(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	args := []string{"issue", "list", "--json", "number,title,url"}
	args = append(args, extraArgs...)

	cmd := commandContext(ctx, "gh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		// Check for invalid options/flags first (these typically come with exit code 1)
		if strings.Contains(stderrStr, "unknown flag") || strings.Contains(stderrStr, "invalid") {
			// Return just the error message for usage errors
			return nil, fmt.Errorf("%s", stderrStr)
		}
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "auth") {
			return nil, fmt.Errorf("gh error: ensure you're authenticated ('gh auth login') and running inside a GitHub repo")
		}
		if strings.Contains(stderrStr, "not found") || strings.Contains(stderrStr, "repository") {
			return nil, fmt.Errorf("gh error: repository not found or not set")
		}
		return nil, fmt.Errorf("gh error: %s", stderrStr)
	}

	var issues []model.IssueListItem
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	return issues, nil
}

func ListClosedIssues() ([]model.IssueListItem, error) {
	return ListIssues([]string{"--state", "closed"})
}
