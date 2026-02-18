package model

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

type Frontmatter struct {
	Title     string `yaml:"title,omitempty"`
	Parent    *int   `yaml:"parent,omitempty"`
	BlockedBy []int  `yaml:"blocked_by,omitempty"`
}

type ErrorType int

const (
	ExitSuccess ErrorType = 0
	ExitUsage   ErrorType = 1
	ExitEnv     ErrorType = 2
	ExitIO      ErrorType = 3
)

type ExitError struct {
	Code    ErrorType
	Message string
	Err     error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func NewUsageError(msg string) *ExitError {
	return &ExitError{Code: ExitUsage, Message: msg}
}

func NewEnvError(msg string, err error) *ExitError {
	return &ExitError{Code: ExitEnv, Message: msg, Err: err}
}

func NewIOError(msg string, err error) *ExitError {
	return &ExitError{Code: ExitIO, Message: msg, Err: err}
}

var numericRegex = regexp.MustCompile(`^[0-9]+$`)

func IsNumeric(s string) bool {
	return numericRegex.MatchString(s)
}

func NormalizeIssueNumbers(values []int) []int {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		normalized = append(normalized, value)
	}

	if len(normalized) == 0 {
		return nil
	}

	sort.Ints(normalized)

	out := normalized[:1]
	for _, value := range normalized[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}

	return out
}

type IssueData struct {
	Title  string       `json:"title"`
	Body   string       `json:"body"`
	Parent *IssueParent `json:"parent"`
}

type IssueParent struct {
	Number int `json:"number"`
}

type IssueListItem struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

var (
	ErrMissingFile          = errors.New("file not found")
	ErrMalformedFrontmatter = errors.New("malformed frontmatter")
)
