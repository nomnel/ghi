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

var (
	linkParent string
)

var linkCmd = &cobra.Command{
	Use:   "link <child-issue-number> --parent <parent-issue-number>",
	Short: "Link an issue as a child of another and update local frontmatter",
	Args:  cobra.ExactArgs(1),
	RunE:  runLink,
}

func init() {
	linkCmd.Flags().StringVar(&linkParent, "parent", "", "Parent issue number")
	linkCmd.MarkFlagRequired("parent")

	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	childNumber := strings.TrimSpace(args[0])
	parentNumber := strings.TrimSpace(linkParent)

	if !model.IsNumeric(childNumber) || !model.IsNumeric(parentNumber) {
		return model.NewUsageError("Usage: ghi link <child-issue-number> --parent <parent-issue-number>")
	}

	if childNumber == parentNumber {
		return model.NewUsageError("child and parent issue numbers must differ")
	}

	issuePath := filepath.Join(issuesDir, fmt.Sprintf("%s.md", childNumber))

	stat, err := os.Stat(issuePath)
	if err != nil {
		if os.IsNotExist(err) {
			return model.NewIOError(fmt.Sprintf("%s not found. Run 'ghi pull %s' first", issuePath, childNumber), nil)
		}
		return model.NewIOError("failed to stat local issue file", err)
	}

	alreadyLinked, err := ensureParentFrontmatter(issuePath, parentNumber)
	if err != nil {
		return err
	}

	if alreadyLinked {
		fmt.Printf("%s already linked to parent #%s. No changes made.\n", issuePath, parentNumber)
		return nil
	}

	if err := gh.AddSubIssue(childNumber, parentNumber); err != nil {
		return model.NewEnvError("", err)
	}

	if err := updateParentFrontmatter(issuePath, parentNumber, stat.Mode().Perm()); err != nil {
		return err
	}

	fmt.Printf("Linked issue #%s as child of #%s and updated %s\n", childNumber, parentNumber, issuePath)
	return nil
}

func ensureParentFrontmatter(issuePath, parentNumber string) (bool, error) {
	raw, err := os.ReadFile(issuePath)
	if err != nil {
		return false, model.NewIOError("failed to read local issue file", err)
	}

	fm, _, err := filefmt.DecodeMarkdown(raw)
	if err != nil {
		return false, model.NewIOError("failed to parse markdown", err)
	}

	if fm.Parent == parentNumber {
		return true, nil
	}

	return false, nil
}

func updateParentFrontmatter(issuePath, parentNumber string, perm os.FileMode) error {
	raw, err := os.ReadFile(issuePath)
	if err != nil {
		return model.NewIOError("failed to read local issue file", err)
	}

	fm, body, err := filefmt.DecodeMarkdown(raw)
	if err != nil {
		return model.NewIOError("failed to parse markdown", err)
	}

	fm.Parent = parentNumber

	content, err := filefmt.EncodeMarkdown(*fm, body)
	if err != nil {
		return model.NewIOError("failed to encode markdown", err)
	}

	if err := filefmt.AtomicWriteFile(issuePath, content, perm); err != nil {
		return model.NewIOError("failed to write file", err)
	}

	return nil
}
