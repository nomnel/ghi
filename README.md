# ghi - GitHub Issue Sync Tool

A simple Go CLI tool to sync GitHub Issues with local markdown files, enabling offline editing and version control of issues.

## Features

- **Pull issues**: Download GitHub issues to local markdown files with YAML frontmatter
- **Push changes**: Update GitHub issues from edited local files
- **Close/Reopen issues**: Change issue state directly from the command line
- **Simple format**: Clean markdown files with YAML frontmatter for metadata
- **Atomic operations**: Safe file writes with atomic operations
- **GitHub CLI integration**: Uses the authenticated `gh` CLI for all GitHub operations

## Installation

### Prerequisites

- Go 1.25 or higher
- [GitHub CLI (`gh`)](https://cli.github.com/) installed and authenticated

### Install with go install

```bash
go install github.com/nomnel/ghi/cmd/ghi@latest
```

### Build from source

```bash
git clone https://github.com/nomnel/ghi.git
cd ghi
go build -o ghi cmd/ghi/main.go
```

## Usage

### Pull an issue

Download a GitHub issue to a local markdown file:

```bash
ghi pull 42
# Saved to issues/42.md
```

### Push changes

Update a GitHub issue from a local markdown file:

```bash
ghi push 42
# Updated issue #42 from issues/42.md
```

### Show differences

Compare a local issue file with the remote GitHub issue:

```bash
ghi diff 42
# Shows differences between issues/42.md and GitHub issue #42
```

The diff shows:
- Title changes (if any)
- Body content differences in unified diff format
- Uses color output for better readability (green for additions, red for deletions)

### Close an issue

Close a GitHub issue:

```bash
ghi close 42
# Closed issue #42.
```

### Reopen an issue

Reopen a closed GitHub issue:

```bash
ghi reopen 42
# Reopened issue #42.
```

## File Format

Issues are stored as markdown files with YAML frontmatter:

```markdown
---
title: Issue title here
---
Issue body content here...
```

## Directory Structure

- Issues are stored in the `issues/` directory (created automatically)
- Files are named `{issue-number}.md`
- Files are overwritten on pull operations
- Push operations read the local file and update the remote issue

## Exit Codes

- `0` - Success
- `1` - Usage/validation error (e.g., non-numeric issue number)
- `2` - Environment/dependency error (e.g., `gh` not authenticated, not in a repo)
- `3` - I/O/parse error (e.g., file not found, malformed YAML)

## Project Structure

```
cmd/ghi/main.go           # CLI entry point with Cobra commands
internal/gh/gh.go         # GitHub CLI wrapper functions
internal/filefmt/md.go    # Markdown/YAML frontmatter handling
internal/model/types.go   # Data structures and error types
```

## Development

See [spec.md](spec.md) for the detailed specification.

### Dependencies

- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) - YAML parsing

### Building

```bash
go build -o ghi cmd/ghi/main.go
```

### Testing

The tool includes comprehensive error handling and validation. Test with:

```bash
# Test invalid input
./ghi pull abc  # Should fail with usage error

# Test missing file
./ghi push 999  # Should fail with file not found error
```

## License

MIT

## Contributing

Pull requests welcome! Please read the [specification](spec.md) before contributing.