# GHI — GitHub Issue Sync Tool (Spec)

A simple Go CLI to **pull** and **push** GitHub Issues using the authenticated `gh` CLI, storing each issue as a markdown file with YAML frontmatter.

---

## 1. Goals & Scope

* Provide two subcommands:

  * `ghi pull <issue-number>`: Download the issue (title, body) from the **current repository** and save to `issues/{number}.md`.
  * `ghi push <issue-number>`: Read `issues/{number}.md` and update the remote issue’s **title** and **body** accordingly.
* File format: Markdown with **YAML frontmatter** (only `title` for now) followed by the raw issue body.
* Overwrite local files on pull; overwrite remote content on push.
* Create `issues/` directory if missing.
* Use `gh` CLI (must be pre-authenticated via `gh auth login`) and `cobra` for CLI scaffolding.
* Use a proper YAML library for parsing (e.g., `gopkg.in/yaml.v3`) to enable future metadata extensions.

---

## 2. CLI Design

```
ghi [command]

Commands:
  pull <issue-number>   Fetch issue from current repo and write to issues/{n}.md
  push <issue-number>   Update issue in current repo from issues/{n}.md
  help                  Show help

Global flags (future-friendly, not required now):
  --version             Print version
```

**Arguments**

* `<issue-number>` must be a non-empty **numeric** string (`^[0-9]+$`).

**Exit codes**

* `0`: success
* `1`: usage / validation error (e.g., non-numeric issue)
* `2`: environment or dependency error (e.g., `gh` missing / not authenticated / not a repo)
* `3`: IO / parse error (file missing, YAML invalid, frontmatter malformed)

---

## 3. Dependencies

* Go ≥ 1.25
* `github.com/spf13/cobra` (CLI)
* `gopkg.in/yaml.v3` (YAML frontmatter parsing/encoding)

**Suggested `go.mod` (example):**

```go
module github.com/yourname/ghi

go 1.25

require (
  github.com/spf13/cobra v1.8.0
  gopkg.in/yaml.v3 v3.0.1
)
```

---

## 4. External Tools & Assumptions

* `gh` CLI is installed and authenticated (`gh auth status` OK).
* Command executes within a GitHub repo directory recognized by `gh` (or user has configured a default via `gh`).
* Uses `gh` defaults for host/repo resolution; no `--repo` flag for now.

---

## 5. Directory & File Conventions

* Local directory: `issues/` (relative to current working directory).
* Filename: `issues/{issue-number}.md` (UTF-8).
* Overwrite behavior: always overwrite on pull; push reads existing file and updates remote.

**Markdown file structure**

```markdown
---
title: <string>   # YAML frontmatter; only `title` for now
---
<raw issue body, copied as-is>
```

* A single newline after the closing `---` before the body.
* Body is unmodified (no wrapping/sanitization).

---

## 6. Data Model

```go
type Frontmatter struct {
    Title string `yaml:"title,omitempty"`
}
```

* For future extensibility, accept unknown YAML keys but **only** use `title` for now.

---

## 7. Command Behaviors

### 7.1 `ghi pull <issue-number>`

**Purpose:** Fetch remote issue and write `issues/{n}.md`.

**Steps:**

1. Validate `<issue-number>` (numeric).
2. Ensure `issues/` exists: `os.MkdirAll("issues", 0o755)`.
3. Invoke `gh`:

   * `gh issue view <n> --json title,body`
4. Parse JSON result:

   * Extract `title` (string) and `body` (string; may be empty).
5. Build YAML frontmatter using `yaml.Encoder` to ensure proper quoting/escaping.
6. Write to a temp file in `issues/` then `os.Rename` to `issues/{n}.md` (atomic-ish write):

   ```
   ---\n
   <YAML-encoded frontmatter>
   ---\n
   <body bytes exactly as returned>
   ```
7. On success, print: `Saved to issues/{n}.md`.

**Errors & messages:**

* If `gh` not found / not authenticated / repo not detected / issue not found:

  * Exit `2` with succinct message, e.g.,
    `gh error: issue not found or repo not set. Authenticate with 'gh auth login' and run inside a repo.`
* If write fails: exit `3` with the underlying error.

---

### 7.2 `ghi push <issue-number>`

**Purpose:** Update remote issue’s title and body from `issues/{n}.md`.

**Steps:**

1. Validate `<issue-number>` (numeric).
2. Read `issues/{n}.md`; if missing, exit `3` with:
   `issues/{n}.md not found. Run 'ghi pull {n}' first.`
3. Split frontmatter and body:

   * File must start with a line exactly `---` (optionally followed by CR or spaces tolerance).
   * Read until the next line matching `---` (that closes the frontmatter).
   * Everything after the closing `---` (first newline after it) is the body.
   * If closing delimiter not found: exit `3` (malformed frontmatter).
4. Parse frontmatter YAML to `Frontmatter` using `yaml.v3`.
5. Create a temp file containing the body (verbatim) for `--body-file`.
6. Build `gh` command:

   * If `title` present and non-empty:
     `gh issue edit <n> --title "<title>" --body-file <temp>`
   * Else:
     `gh issue edit <n> --body-file <temp>`
7. Execute; on non-zero status, exit `2` with succinct `gh` error.
8. On success, print: `Updated issue #{n} from issues/{n}.md`.

**Notes:**

* Preserve body exactly; do not transform line endings. (Read/write as \[]byte.)
* Accept empty body (clears remote body).

---

## 8. Implementation Notes

* Use `os/exec` with `exec.CommandContext` and a timeout (e.g., 30s) for `gh` calls.
* Capture `stdout` (JSON for pull) and `stderr`; include `stderr` snippet in error messages where helpful.
* JSON structure from `gh issue view`:

  ```json
  { "title": "...", "body": "..." }
  ```
* Ensure a newline after YAML closing `---` before writing the body.
* Atomic writes:

  * `tmp, err := os.CreateTemp("issues", fmt.Sprintf(".%d-*.md", issueNumber))`
  * Write all bytes; `tmp.Sync()`; `tmp.Close()`; `os.Rename(tmp.Name(), finalPath)`
* Cross-platform:

  * Avoid shell features; invoke `gh` directly (`exec.LookPath("gh")`).
* Encoding:

  * Treat all text as UTF-8; do not modify.

---

## 9. Project Layout

```
/cmd/ghi/main.go            // cobra root + subcommands wiring
/internal/gh/gh.go          // wrappers to call gh and parse JSON results
/internal/filefmt/md.go     // read/write markdown with YAML frontmatter
/internal/model/types.go    // Frontmatter struct, helpers
```

---

## 10. Pseudocode

**pull**

```go
func runPull(issue string) error {
  if !isNumeric(issue) { return usageErr }
  ensureDir("issues", 0o755)
  js, err := ghView(issue) // calls: gh issue view <n> --json title,body
  if err != nil { return envErr(err) }
  title := js.Title
  body  := js.Body // string
  fm := Frontmatter{Title: title}
  b := encodeMarkdown(fm, []byte(body)) // writes '---\n', YAML, '---\n', body
  return atomicWriteFile(filepath.Join("issues", issue+".md"), b, 0o644)
}
```

**push**

```go
func runPush(issue string) error {
  if !isNumeric(issue) { return usageErr }
  path := filepath.Join("issues", issue+".md")
  raw, err := os.ReadFile(path)
  if err != nil { return fmt.Errorf("%s not found. Run 'ghi pull %s' first", path, issue) }

  fm, body, err := decodeMarkdown(raw) // validates leading/closing '---'
  if err != nil { return parseErr(err) }

  tmp, _ := os.CreateTemp("", "ghi-body-*")
  defer os.Remove(tmp.Name())
  tmp.Write(body)
  tmp.Close()

  if strings.TrimSpace(fm.Title) != "" {
     return ghEdit(issue, "--title", fm.Title, "--body-file", tmp.Name())
  }
  return ghEdit(issue, "--body-file", tmp.Name())
}
```

---

## 11. Validation & Error Messages

* Non-numeric argument:
  `Usage: ghi <pull|push> <issue-number>`
* Missing file on push:
  `issues/{n}.md not found. Run 'ghi pull {n}' first.`
* Malformed frontmatter:
  `Invalid frontmatter in issues/{n}.md: missing closing '---'.`
* `gh` not installed:
  `gh CLI not found. Install GitHub CLI and run 'gh auth login'.`
* `gh` not authenticated / repo not detected / not found: show concise `stderr` from gh and guidance:

  * `gh error: verify authentication ('gh auth status') and run inside a Git repo.`
* IO errors include the underlying `error.Error()`.

---

## 12. Examples

```bash
# Pull issue #42 into issues/42.md
ghi pull 42
# -> Saved to issues/42.md

# Edit issues/42.md in your editor, then push changes:
ghi push 42
# -> Updated issue #42 from issues/42.md
```

---

## 13. Acceptance Criteria

* **Pull**

  * Creates `issues/` if absent.
  * Writes `issues/{n}.md` with valid YAML frontmatter (`title`) and raw body.
  * Overwrites an existing file.
  * Succeeds for empty body; preserves all formatting.

* **Push**

  * Reads `issues/{n}.md`, parses frontmatter.
  * Updates remote issue title iff `title` present & non-empty.
  * Updates remote body exactly to file body.
  * Errors clearly if file missing or frontmatter malformed.

* **General**

  * Exits with proper codes.
  * No modification of body text.
  * Works on macOS, Linux, Windows with `gh` installed and logged in.

---

## 14. Future Extensions (non-blocking)

* Support `--repo <owner/name>` to operate outside a git repo.
* Additional frontmatter fields (`labels`, `assignees`, `milestone`, etc.).
* Round-trip fidelity tests (pull → push idempotence).
* `ghi diff <n>` to compare local vs remote.

---

## 15. Security & Privacy

* No token handling; relies on `gh` auth.
* Writes only to `issues/` subdirectory.
* Avoid logging sensitive content; print only high-level success messages.
