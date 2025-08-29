package model

import (
	"errors"
	"fmt"
	"regexp"
)

type Frontmatter struct {
	Title string `yaml:"title,omitempty"`
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

type IssueData struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type IssueListItem struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

var (
	ErrMissingFile        = errors.New("file not found")
	ErrMalformedFrontmatter = errors.New("malformed frontmatter")
)