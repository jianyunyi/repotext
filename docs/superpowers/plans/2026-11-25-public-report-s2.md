# PUBLIC-2 Read-Only Sharing Implementation Plan

> **For agentic workers:** Execute this plan inline in the current workspace. Do not commit or open a listener; the user authorized implementation but not repository integration or public exposure.

**Goal:** Add an independently mountable anonymous read-only report handler at `/r/<task-id>` without exposing mutation routes or full logs.

**Architecture:** Keep the existing local/LAN `API.Handler()` unchanged. Add `API.PublicHandler()` with a narrow route table, a public report projection loaded through existing task/report store methods, and a template-backed page that reuses the safe DOM report renderer.

**Tech Stack:** Go `net/http`, `html/template`, embedded HTML/CSS/JavaScript, SQLiteStore, existing `domain.Report` renderer, and Go tests.

---

### Task 1: Define the public report contract with failing tests

**Files:**
- Create: `internal/control/public_report_test.go`
- Create: `internal/control/public_handler_test.go`
- Create: `internal/control/public_report.go`

- [x] **Step 1: Add tests for the public projection.**

  Cover a 32-hex task ID matching the existing 16-byte random ID format, terminal task with a stored report, running task without a report, expired task, malformed ID, unknown ID, and a report containing a private error message plus long output. Assert the public response omits private task error fields, clamps diagnostics to 4096 bytes, preserves truncation, and returns 410 for a finished task older than seven days.

- [x] **Step 2: Run the focused tests and confirm they fail.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicReport' -v`

  Expected: FAIL because `API.PublicHandler` and the public projection are not implemented.

- [x] **Step 3: Implement the public projection loader.**

  Add a store interface using `Get` and `GetReport`, a seven-day TTL, strict 32-character lowercase hexadecimal ID validation, a public task struct, a public response envelope, and a copy-based report sanitizer. Keep diagnostics only for failed, build-failed, or incomplete items and clear full output from successful/skipped cases.

- [x] **Step 4: Run the focused tests and confirm they pass.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicReport' -v`

  Expected: PASS for projection, expiry, ID validation, and redaction.

### Task 2: Add the isolated public HTTP handler

**Files:**
- Create: `internal/control/public_handler.go`
- Create: `internal/control/public_handler_test.go`

- [x] **Step 1: Add failing route and header tests.**

  Assert that a non-loopback request without CSRF or pairing can read the public API and page. Assert that log, task mutation, session, unknown, and malformed routes do not return success. Assert CSP, no-store, nosniff, no-referrer, Open Graph metadata, and escaped task data.

- [x] **Step 2: Run the focused handler tests and confirm they fail.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicHandler' -v`

  Expected: FAIL because the public handler and page do not exist.

- [x] **Step 3: Implement `API.PublicHandler()`.**

  Register only `GET /r/{id}`, `GET /api/v1/public/reports/{id}`, and public static assets. Use `html/template` for escaped title, description, and task ID. Apply restrictive security headers and never call or wrap existing mutation routes.

- [x] **Step 4: Run the focused handler tests and confirm they pass.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicHandler' -v`

  Expected: PASS for route isolation, headers, and metadata escaping.

### Task 3: Add the public page and safe client bootstrapping

**Files:**
- Create: `internal/control/web/public.html`
- Create: `internal/control/web/public.js`
- Create: `internal/control/web/public.css`
- Modify: `internal/control/public_handler_test.go` for asset assertions.

- [x] **Step 1: Add failing page safety assertions.**

  Assert the page loads the existing `report.js` renderer, contains a report mount point and public task ID data attribute, has no `innerHTML`, mutation endpoint, CSRF token, or pairing control, and uses DOM text APIs for status/error messages.

- [x] **Step 2: Run the page tests and confirm they fail.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicPage' -v`

  Expected: FAIL because the public page assets do not exist.

- [x] **Step 3: Implement the page.**

  Create a read-only page that fetches the public API, renders status and error text through DOM APIs, calls `RepoTestReport.render` for a report, and presents expiry/unavailable states without write controls. Keep repository-derived content out of HTML string concatenation.

- [x] **Step 4: Run the page tests and confirm they pass.**

  Run: `go test -p=1 -count=1 ./internal/control -run 'TestPublicPage' -v`

  Expected: PASS for content, assets, and no-write UI assertions.

### Task 4: Integrate documentation and verify the repository

**Files:**
- Modify: `docs/publicization-stage-report.md`
- Modify: `docs/superpowers/specs/2026-11-25-public-report-s2-design.md` only with verified evidence.

- [x] **Step 1: Run formatting and focused verification.**

  Run `gofmt` on new Go files, then `go test -p=1 -count=1 ./internal/control`.

- [x] **Step 2: Run repository verification with a workspace cache.**

  Run `$env:GOCACHE=(Resolve-Path 'work/go-cache').Path; go test -p=1 -count=1 ./...`, then `go vet ./...`, `go build ./...`, and `git diff --check`.

- [x] **Step 3: Update the stage report from actual evidence.**

  Record PUBLIC-2 route, projection, expiry, CSP, no-write, and no-full-log checks. Keep PUBLIC-1 marked as repairing until the GitHub Windows CI rerun is observed. State that no public listener was added and PUBLIC-3/4/5/6 remain pending.

- [x] **Step 4: Review the final diff for scope.**

  Confirm only PUBLIC-2 code, tests, assets, design/plan evidence, and the existing Windows fix are present. Do not change deployment files, ports, runner enablement, credentials, or production data.
