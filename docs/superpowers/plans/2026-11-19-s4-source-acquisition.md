# S4 Immutable Source Acquisition Implementation Plan

> **For agentic workers:** Follow the plan task by task using inline execution. Each step is checked before the next; do not enable task execution as part of this plan.

**Goal:** Fetch a user-confirmed GitHub commit archive and extract only bounded regular files into a private scratch directory without following redirects outside GitHub or accepting archive path/link escapes.

**Architecture:** A small `internal/runner` module validates the canonical GitHub repository and 40-character commit SHA, asks the GitHub archive endpoint for that exact SHA, accepts at most one HTTPS redirect to `codeload.github.com`, bounds compressed/decompressed bytes and entry count, and writes only regular files/directories under a new empty destination. It does not clone, run git hooks, invoke a shell, fetch Go modules, or execute repository contents.

**Tech Stack:** Go standard library (`net/http`, `archive/tar`, `compress/gzip`, `os`, `path/filepath`); no new dependencies.

---

## File Map

- Create `internal/runner/archive.go`: archive-fetch interface, URL/SHA validation, bounded response handling, safe tar extraction.
- Create `internal/runner/archive_test.go`: HTTP redirect, status, byte-limit, path, link, file-count, and extraction tests using `httptest` and generated tarballs.
- Modify `docs/project-design.md`: record archive byte and entry limits and source-acquisition state.
- Modify `docs/stage-report.md`: record exact commands, outcomes, remaining AC-008 gates, and SHA-256 checkpoint.

## Limits Frozen for This Slice

- Compressed HTTP archive: 100 MiB maximum (matches the existing repository-size upper bound).
- Decompressed tar stream: 100 MiB maximum.
- Entries: 100,000 maximum; regular-file payload total also cannot exceed 100 MiB.
- Archive entries may be regular files or directories only. Symlinks, hardlinks, devices, FIFOs, absolute paths, `..`, backslashes, duplicate files, and paths outside the single GitHub top-level directory are rejected.
- Destination must be a new empty directory under the service's private task scratch parent. The source directory and children use `0755` so a non-root sandbox UID can read a read-only bind mount; regular files use `0644` and retain an executable bit as `0755`. The containing task scratch parent remains `0700`, so public repository contents are not exposed to other host users.
- HTTP redirect count is at most one and the redirect target must be HTTPS `codeload.github.com` with no userinfo or explicit port. Other redirects fail closed.

## Task 1: Test Immutable Commit Archive Fetching

**Files:** `internal/runner/archive_test.go`

- [x] **Step 1: Write tests for input and redirect validation.** Tests reject invalid repository/SHA before network access; accept one fixed codeload redirect; reject localhost, HTTP, userinfo, non-default port, second redirect, and non-200 archive responses.
- [x] **Step 2: Run the tests and observe expected failures.** `go test ./internal/runner -run 'TestArchive(Fetcher|Extract)' -count=1` initially failed to compile because `ArchiveFetcher`, `archiveLimits`, and `extractTarGz` were absent.
- [x] **Step 3: Write extraction fixtures.** Fixtures cover regular files; traversal, absolute paths, backslashes, symlink, hardlink, FIFO, duplicate file, multiple roots, compressed/expanded/payload/file-count caps, and non-empty destination handling.
- [x] **Step 4: Run extraction tests and observe expected failures.** The same initial compile failure confirmed those tests referenced the missing implementation.

## Task 2: Implement the Bounded Fetcher and Extractor

**Files:** `internal/runner/archive.go`, `internal/runner/archive_test.go`

- [x] **Step 1: Define the smallest injectable seam.** `ArchiveFetcher` carries an HTTP client plus private fixed GitHub API/redirect host and bounded archive limits; same-package tests use a deterministic `RoundTripper`.
- [x] **Step 2: Implement `FetchAndExtract(ctx context.Context, repoURL, commitSHA, destination string) error`.** Validates the repository and full SHA before network access, requests that SHA's tarball, verifies a single allowed redirect path, status and declared length, then enforces compressed/decompressed byte limits.
- [x] **Step 3: Extract safely.** Requires a new destination; strips one common top-level directory; rejects unsafe paths/types/duplicates; caps entry count and bytes; uses `O_EXCL`; cleans the destination on failure. Directories/files are `0755`/`0644` with archive executable bits retained as `0755` under a private parent.
- [x] **Step 4: Run focused tests.** `gofmt -w internal/runner/archive.go internal/runner/archive_test.go` and `go test ./internal/runner -run 'TestArchive(Fetcher|Extract)' -count=1` passed.
- [ ] **Step 5: Run the runner package suite.** Run `go test ./internal/runner -count=1`; expected result: `ok repotest/internal/runner`.

## Task 3: Record Evidence and Security Limits

**Files:** `docs/project-design.md`, `docs/stage-report.md`

- [ ] **Step 1: Record source-acquisition policy.** State the fixed SHA endpoint, one validated redirect, 100 MiB compressed/decompressed/payload caps, 100,000-entry cap, regular-files-only rule, and that no repository code executes during acquisition.
- [ ] **Step 2: Record test evidence.** Include the focused and package-wide Go commands, actual exit status, representative malicious archive cases, source file SHA-256, and note that no real user's repository was fetched for this unit-only verification.
- [ ] **Step 3: Run local repository checks.** Run `go test ./... -count=1`, `go vet ./...`, and `go build ./...`; expected exit code 0 for each.
- [ ] **Step 4: Keep AC-008 open.** Explicitly leave module proxy, no-network test execution, CPU/PID pressure testing, persistent disk quota, cancellation/restart cleanup, and production wiring NOT RUN until their own implementation and Linux acceptance.

## Plan Self-Review

- Scope is one independently testable module: fixed-commit source acquisition; module proxy, executor, report parsing, and worker integration are separate follow-on tasks.
- No archive entry is allowed to create a link or special file, so later container bind mounting cannot follow a repository-provided symlink to another host path.
- The repository URL and full SHA are both validated before network access; the redirect target is separately restricted.
- All payload and expansion dimensions are bounded; any failure cleans the private destination.
- No new dependency or repository code execution is introduced by this slice.
