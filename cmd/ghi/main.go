package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/gh"
	"github.com/nomnel/ghi/internal/model"
	"github.com/spf13/cobra"
)

const issuesDir = "issues"

var rootCmd = &cobra.Command{
	Use:   "ghi",
	Short: "GitHub Issue Sync Tool",
	Long:  "A simple CLI to pull and push GitHub Issues using the authenticated gh CLI, storing each issue as a markdown file with YAML frontmatter.",
}

var pullCmd = &cobra.Command{
	Use:   "pull <issue-number>",
	Short: "Fetch issue from current repo and write to issues/{n}.md",
	Args:  cobra.ExactArgs(1),
	RunE:  runPull,
}

var pushCmd = &cobra.Command{
	Use:   "push <issue-number>",
	Short: "Update issue in current repo from issues/{n}.md",
	Args:  cobra.ExactArgs(1),
	RunE:  runPush,
}

var diffCmd = &cobra.Command{
	Use:   "diff <issue-number> [--] [EXTRA_GIT_DIFF_ARGS...]",
	Short: "Compare local issues/{n}.md with remote GitHub Issue",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runDiff,
}

var createCmd = &cobra.Command{
	Use:   "create <issue-title>",
	Short: "Create a new GitHub Issue and pull it locally",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

var closeCmd = &cobra.Command{
	Use:   "close <issue-number>",
	Short: "Close the specified GitHub issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runClose,
}

var reopenCmd = &cobra.Command{
	Use:   "reopen <issue-number>",
	Short: "Reopen the specified GitHub issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runReopen,
}

func init() {
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(closeCmd)
	rootCmd.AddCommand(reopenCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *model.ExitError
		if e, ok := err.(*model.ExitError); ok {
			exitErr = e
		} else {
			exitErr = &model.ExitError{Code: model.ExitIO, Message: err.Error()}
		}
		
		fmt.Fprintln(os.Stderr, exitErr.Error())
		os.Exit(int(exitErr.Code))
	}
}

func runPull(cmd *cobra.Command, args []string) error {
	issueNumber := args[0]
	
	if !model.IsNumeric(issueNumber) {
		return model.NewUsageError("Usage: ghi pull <issue-number>")
	}
	
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return model.NewIOError("failed to create issues directory", err)
	}
	
	issue, err := gh.ViewIssue(issueNumber)
	if err != nil {
		return model.NewEnvError("", err)
	}
	
	fm := model.Frontmatter{Title: issue.Title}
	
	content, err := filefmt.EncodeMarkdown(fm, []byte(issue.Body))
	if err != nil {
		return model.NewIOError("failed to encode markdown", err)
	}
	
	filePath := filepath.Join(issuesDir, fmt.Sprintf("%s.md", issueNumber))
	
	if err := filefmt.AtomicWriteFile(filePath, content, 0o644); err != nil {
		return model.NewIOError("failed to write file", err)
	}
	
	fmt.Printf("Saved to %s\n", filePath)
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	issueNumber := args[0]
	
	if !model.IsNumeric(issueNumber) {
		return model.NewUsageError("Usage: ghi push <issue-number>")
	}
	
	filePath := filepath.Join(issuesDir, fmt.Sprintf("%s.md", issueNumber))
	
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return model.NewIOError(fmt.Sprintf("%s not found. Run 'ghi pull %s' first", filePath, issueNumber), nil)
		}
		return model.NewIOError("failed to read file", err)
	}
	
	fm, body, err := filefmt.DecodeMarkdown(raw)
	if err != nil {
		if strings.Contains(err.Error(), "malformed frontmatter") {
			return model.NewIOError(fmt.Sprintf("Invalid frontmatter in %s", filePath), err)
		}
		return model.NewIOError("failed to parse markdown", err)
	}
	
	tmpFile, err := gh.CreateTempBodyFile(body)
	if err != nil {
		return model.NewIOError("failed to create temp file", err)
	}
	defer os.Remove(tmpFile)
	
	if err := gh.EditIssue(issueNumber, fm.Title, tmpFile); err != nil {
		return model.NewEnvError("", err)
	}
	
	fmt.Printf("Updated issue #%s from %s\n", issueNumber, filePath)
	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	issueNumber := args[0]
	
	if !model.IsNumeric(issueNumber) {
		return model.NewUsageError("Usage: ghi diff <issue-number> [--] [EXTRA_GIT_DIFF_ARGS...]")
	}
	
	localPath := filepath.Join(issuesDir, fmt.Sprintf("%s.md", issueNumber))
	
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return model.NewIOError(fmt.Sprintf("%s not found. Run 'ghi pull %s' first.", localPath, issueNumber), nil)
		}
		return model.NewIOError("failed to check local file", err)
	}
	
	issue, err := gh.ViewIssue(issueNumber)
	if err != nil {
		return model.NewEnvError("", err)
	}
	
	tmpDir := filepath.Join(issuesDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return model.NewIOError("failed to create temp directory", err)
	}
	
	tmpFile, err := os.CreateTemp(tmpDir, fmt.Sprintf("remote-%s-*.md", issueNumber))
	if err != nil {
		return model.NewIOError("failed to create temp file", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	
	fm := model.Frontmatter{Title: issue.Title}
	content, err := filefmt.EncodeMarkdown(fm, []byte(issue.Body))
	if err != nil {
		tmpFile.Close()
		return model.NewIOError("failed to encode remote markdown", err)
	}
	
	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return model.NewIOError("failed to write temp file", err)
	}
	
	if err := tmpFile.Close(); err != nil {
		return model.NewIOError("failed to close temp file", err)
	}
	
	extraArgs := args[1:]
	dashIndex := -1
	for i, arg := range extraArgs {
		if arg == "--" {
			dashIndex = i
			break
		}
	}
	
	if dashIndex >= 0 {
		extraArgs = extraArgs[dashIndex+1:]
	}
	
	exitCode, err := gh.RunGitDiff(tmpPath, localPath, extraArgs)
	if err != nil {
		return model.NewEnvError("", err)
	}
	
	switch exitCode {
	case 0:
		fmt.Printf("No differences: %s matches remote.\n", localPath)
		return nil
	case 1:
		os.Exit(1)
		return nil
	default:
		return model.NewEnvError(fmt.Sprintf("git diff failed with exit code %d", exitCode), nil)
	}
}

func runCreate(cmd *cobra.Command, args []string) error {
	title := strings.TrimSpace(args[0])
	
	if title == "" {
		return model.NewUsageError("Usage: ghi create <issue-title>")
	}
	
	issueNumber, err := gh.CreateIssue(title)
	if err != nil {
		return model.NewEnvError("", err)
	}
	
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return model.NewIOError(fmt.Sprintf("Issue #%d created on GitHub but failed to create local directory", issueNumber), err)
	}
	
	issue, err := gh.ViewIssue(fmt.Sprintf("%d", issueNumber))
	if err != nil {
		return model.NewIOError(fmt.Sprintf("Issue #%d created on GitHub but failed to fetch details", issueNumber), err)
	}
	
	fm := model.Frontmatter{Title: issue.Title}
	
	content, err := filefmt.EncodeMarkdown(fm, []byte(issue.Body))
	if err != nil {
		return model.NewIOError(fmt.Sprintf("Issue #%d created on GitHub but failed to encode markdown", issueNumber), err)
	}
	
	filePath := filepath.Join(issuesDir, fmt.Sprintf("%d.md", issueNumber))
	
	if err := filefmt.AtomicWriteFile(filePath, content, 0o644); err != nil {
		return model.NewIOError(fmt.Sprintf("Issue #%d created on GitHub but failed to write local file", issueNumber), err)
	}
	
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return model.NewIOError(fmt.Sprintf("Issue #%d created and saved locally but failed to resolve absolute path", issueNumber), err)
	}
	
	fmt.Println(absPath)
	return nil
}

func runClose(cmd *cobra.Command, args []string) error {
	issueNumber := args[0]
	
	if !model.IsNumeric(issueNumber) {
		return model.NewUsageError("Usage: ghi close <issue-number>")
	}
	
	if err := gh.CloseIssue(issueNumber); err != nil {
		return model.NewEnvError("", err)
	}
	
	return nil
}

func runReopen(cmd *cobra.Command, args []string) error {
	issueNumber := args[0]
	
	if !model.IsNumeric(issueNumber) {
		return model.NewUsageError("Usage: ghi reopen <issue-number>")
	}
	
	if err := gh.ReopenIssue(issueNumber); err != nil {
		return model.NewEnvError("", err)
	}
	
	return nil
}