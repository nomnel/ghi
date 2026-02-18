package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nomnel/ghi/internal/filefmt"
	"github.com/nomnel/ghi/internal/gh"
	"github.com/nomnel/ghi/internal/model"
	"github.com/spf13/cobra"
)

const issuesDir = "issues"

var addSubIssueFn = gh.AddSubIssue
var addBlockedByFn = gh.AddBlockedBy

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

var linkCmd = &cobra.Command{
	Use:   "link <issue> [--parent <parent>] [--blocked-by <issue> ...]",
	Short: "Link parent and dependency relationships for an issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runLink,
}

var listCmd = &cobra.Command{
	Use:                "list [-- GH_ISSUE_LIST_OPTIONS...]",
	Short:              "List open GitHub Issues with custom formatting",
	Args:               cobra.ArbitraryArgs,
	DisableFlagParsing: true,
	RunE:               runList,
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete local files for closed GitHub issues",
	Args:  cobra.NoArgs,
	RunE:  runPrune,
}

func init() {
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(closeCmd)
	rootCmd.AddCommand(reopenCmd)
	linkCmd.Flags().String("parent", "", "Parent issue number or GitHub issue URL")
	linkCmd.Flags().StringArray("blocked-by", nil, "Issue number or GitHub issue URL that blocks this issue (repeatable)")
	linkCmd.Flags().StringArray("depends-on", nil, "Unsupported option; use --blocked-by")
	_ = linkCmd.Flags().MarkHidden("depends-on")
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(pruneCmd)
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

func issueFrontmatter(issue *model.IssueData) model.Frontmatter {
	fm := model.Frontmatter{Title: issue.Title}
	if issue.Parent != nil {
		parentNumber := issue.Parent.Number
		fm.Parent = &parentNumber
	}

	return fm
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

	fm := issueFrontmatter(issue)

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

	if err := gh.EditIssue(issueNumber, fm.Title(), tmpFile); err != nil {
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

	fm := issueFrontmatter(issue)
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

	fm := issueFrontmatter(issue)

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

func runLink(cmd *cobra.Command, args []string) error {
	const usage = "Usage: ghi link <issue> [--parent <parent>] [--blocked-by <issue> ...]"

	dependsOn, _ := cmd.Flags().GetStringArray("depends-on")
	if len(dependsOn) > 0 {
		return model.NewUsageError(usage + "\n--depends-on is not supported. Use --blocked-by instead.")
	}

	issue, err := normalizeIssueReference(args[0])
	if err != nil {
		return model.NewUsageError(usage)
	}

	parentRaw, _ := cmd.Flags().GetString("parent")
	var parent *int
	if strings.TrimSpace(parentRaw) != "" {
		parentIssue, err := normalizeIssueReference(parentRaw)
		if err != nil {
			return model.NewUsageError(usage)
		}
		parent = &parentIssue
	}

	blockedByRaw, _ := cmd.Flags().GetStringArray("blocked-by")
	blockedBy, err := normalizeIssueReferences(blockedByRaw)
	if err != nil {
		return model.NewUsageError(usage)
	}

	if parent == nil && len(blockedBy) == 0 {
		return model.NewUsageError(usage)
	}

	if parent != nil && issue == *parent {
		return model.NewUsageError("Usage: child and parent issue numbers must differ")
	}

	for _, blocker := range blockedBy {
		if blocker == issue {
			return model.NewUsageError("Usage: --blocked-by cannot include the target issue")
		}
		if parent != nil && blocker == *parent {
			return model.NewUsageError("Usage: --blocked-by cannot include --parent")
		}
	}

	issueNumber := strconv.Itoa(issue)
	filePath := filepath.Join(issuesDir, fmt.Sprintf("%s.md", issueNumber))
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return model.NewIOError(fmt.Sprintf("%s not found. Run 'ghi pull %s' first", filePath, issueNumber), err)
		}
		return model.NewIOError("failed to read issue file", err)
	}

	fm, body, err := filefmt.DecodeMarkdown(raw)
	if err != nil {
		if strings.Contains(err.Error(), "malformed frontmatter") {
			return model.NewIOError(fmt.Sprintf("Invalid frontmatter in %s", filePath), err)
		}
		return model.NewIOError("failed to parse markdown", err)
	}

	existingBlockedBy, err := fm.BlockedBy()
	if err != nil {
		return model.NewIOError(fmt.Sprintf("Invalid frontmatter in %s", filePath), err)
	}

	parentStatus := "not-requested"
	if parent != nil {
		if fm.HasParent(*parent) {
			parentStatus = "unchanged"
		} else {
			parentStatus = "pending"
		}
	}

	existingBlockers := make(map[int]struct{}, len(existingBlockedBy))
	for _, value := range existingBlockedBy {
		existingBlockers[value] = struct{}{}
	}

	blockedByToCreate := make([]int, 0, len(blockedBy))
	for _, value := range blockedBy {
		if _, exists := existingBlockers[value]; !exists {
			blockedByToCreate = append(blockedByToCreate, value)
		}
	}

	blockedByAdded := 0
	anyRemoteSuccess := false

	if parentStatus == "pending" {
		if err := addSubIssueFn(issueNumber, strconv.Itoa(*parent)); err != nil {
			parentStatus = "failed"
			fmt.Println(linkSummaryLine(parentStatus, blockedByAdded, len(blockedBy), anyRemoteSuccess))
			return model.NewEnvError("", err)
		}
		parentStatus = "added"
		anyRemoteSuccess = true
	}

	for _, blocker := range blockedByToCreate {
		if err := addBlockedByFn(issueNumber, strconv.Itoa(blocker)); err != nil {
			fmt.Println(linkSummaryLine(parentStatus, blockedByAdded, len(blockedBy), anyRemoteSuccess))
			return model.NewEnvError("", err)
		}
		blockedByAdded++
		anyRemoteSuccess = true
	}

	if parentStatus == "added" {
		fm.SetParent(*parent)
	}

	if len(blockedBy) > 0 {
		merged := append(append([]int{}, existingBlockedBy...), blockedBy...)
		fm.SetBlockedBy(merged)
	}

	content, err := filefmt.EncodeFrontmatterDoc(fm, body)
	if err != nil {
		return model.NewIOError("issue links updated on GitHub but failed to encode markdown", err)
	}

	if err := filefmt.AtomicWriteFile(filePath, content, 0o644); err != nil {
		return model.NewIOError("issue links updated on GitHub but failed to update local file; manually align frontmatter to restore consistency", err)
	}

	fmt.Println(linkSummaryLine(parentStatus, blockedByAdded, len(blockedBy), false))
	return nil
}

func normalizeIssueReferences(rawReferences []string) ([]int, error) {
	values := make([]int, 0, len(rawReferences))
	for _, raw := range rawReferences {
		value, err := normalizeIssueReference(raw)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}

	return model.NormalizeIssueNumbers(values), nil
}

func normalizeIssueReference(raw string) (int, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return 0, fmt.Errorf("empty issue reference")
	}

	if model.IsNumeric(ref) {
		n, err := strconv.Atoi(ref)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid issue number")
		}
		return n, nil
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return 0, fmt.Errorf("invalid issue URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return 0, fmt.Errorf("unsupported URL scheme")
	}

	host := strings.ToLower(parsed.Host)
	if host != "github.com" && host != "www.github.com" {
		return 0, fmt.Errorf("unsupported issue URL host")
	}

	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) != 4 || pathParts[2] != "issues" || !model.IsNumeric(pathParts[3]) {
		return 0, fmt.Errorf("unsupported issue URL path")
	}

	n, err := strconv.Atoi(pathParts[3])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid issue URL number")
	}

	return n, nil
}

func linkSummaryLine(parentStatus string, blockedByAdded, blockedByRequested int, partialSuccess bool) string {
	partial := "no"
	if partialSuccess {
		partial = "yes"
	}

	return fmt.Sprintf(
		"link summary: parent=%s blocked_by_added=%d/%d partial_success=%s",
		parentStatus,
		blockedByAdded,
		blockedByRequested,
		partial,
	)
}

func runList(cmd *cobra.Command, args []string) error {
	// Find the "--" separator if present
	extraArgs := []string{}
	dashIndex := -1
	for i, arg := range args {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	// Everything after "--" is passed to gh
	if dashIndex >= 0 {
		extraArgs = args[dashIndex+1:]
	}

	issues, err := gh.ListIssues(extraArgs)
	if err != nil {
		// Check if it's an invalid option error from gh
		if strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "unrecognized") {
			return model.NewUsageError(err.Error())
		}
		return model.NewEnvError("", err)
	}

	// Format and output issues
	for i, issue := range issues {
		fmt.Printf("#%d %s\n", issue.Number, issue.Title)
		fmt.Println(issue.URL)
		// Add blank line between issues, but not after the last one
		if i < len(issues)-1 {
			fmt.Println()
		}
	}

	return nil
}

func runPrune(cmd *cobra.Command, args []string) error {
	// Check if issues directory exists
	if _, err := os.Stat(issuesDir); os.IsNotExist(err) {
		return model.NewIOError("issues directory does not exist", nil)
	}

	// Get list of closed issues from GitHub
	closedIssues, err := gh.ListClosedIssues()
	if err != nil {
		return model.NewEnvError("failed to list closed issues", err)
	}

	// If no closed issues, exit silently with success
	if len(closedIssues) == 0 {
		return nil
	}

	// Delete files for each closed issue
	for _, issue := range closedIssues {
		filePath := filepath.Join(issuesDir, fmt.Sprintf("%d.md", issue.Number))

		// Check if file exists before attempting deletion
		if _, err := os.Stat(filePath); err == nil {
			if err := os.Remove(filePath); err != nil {
				return model.NewIOError(fmt.Sprintf("failed to delete %s", filePath), err)
			}
		}
		// If file doesn't exist, continue silently
	}

	// Delete tmp directory if it exists
	tmpDir := filepath.Join(issuesDir, "tmp")
	if _, err := os.Stat(tmpDir); err == nil {
		if err := os.RemoveAll(tmpDir); err != nil {
			return model.NewIOError("failed to delete tmp directory", err)
		}
	}

	// Silent exit on success
	return nil
}
