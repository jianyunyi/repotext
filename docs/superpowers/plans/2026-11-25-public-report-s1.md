# Public Report Stage 1 Implementation Plan

> **For agentic workers:** Use subagent-driven-development for bounded implementation tasks, with independent spec and quality review. Execute sequentially; do not dispatch two writers for shared domain/runner files.

**Goal:** Deliver useful bounded case-level Go reports, honest coverage/failure locations and administrator diagnostics, then validate them against genuine tool output and pinned public repositories.

**Architecture:** Extend existing Report/Package DTOs and runner parsing. Preserve current access and sandbox boundaries; expose a reusable safe report renderer and protected local log read. No new third-party dependencies.

**Baseline:** 10a0bc5 on feat/public-report-s1; approved checklist and design in ../specs/2026-11-25-public-report-s1-design.md. Runtime remains old private preview service. Public concurrency starts at 1.

## Task 1: Backend schema and parser (PR-AC-01/02/03/04)

Files: internal/domain/task.go; internal/runner/report.go or a focused new report source helper; internal/runner/report_test.go and new focused tests; internal/runner/offline.go; internal/runner/docker_executor.go; applicable offline tests.

- [x] Add failing expectations through existing offlineReport and JSON decoding so missing optional fields produce semantic RED, not a syntax/import mistake. Cover nested and parallel cases, missing/late terminal events, panic/build failure, real coverage/no-tests, output truncation and path/link safety.
- [x] Run `go test -count=1 -run 'TestOfflineReport|TestReport' ./internal/runner`; retain the expected RED output in work/public-report-s1.
- [x] Implement the DTO/parser/source mapping and bounded outputs described in the spec. Keep legacy offlineReport usable. Add -cover to the actual offline command and preserve capped dependency stderr on error.
- [x] Focused GREEN, existing runner tests, gofmt, `go vet ./internal/runner ./internal/domain`; record actual outputs and any unresolved container-only check.

## Task 2: Safe report rendering and local diagnostics (PR-AC-05)

Files: internal/control/web/index.html; a reusable web/report.js if justified; internal/control/handler.go; applicable handler/response/browser tests. Use the exact Task 1 JSON contract after its review passes.

- [x] Write RED checks for case details, trusted source links, incomplete/truncated/no-tests/coverage, XSS text and protected GET log route. Extract the existing inline LAN log reader into API.log and retain the current CSRF/local and LAN session boundaries.
- [x] Implement DOM-based report rendering with summary/failure-first/filter-sort/subtests/trust hints. Prefer a small standalone asset for reuse by the next read-only page rather than expanding inline code.
- [x] Run handler/control checks and real browser interaction/screenshot QA using fixture data clearly labelled as fixtures. Existing preview/confirm/cancel/delete/session flow must still work.

## Task 3: Real output corpus and pinned repository evidence (PR-AC-06)

- [x] Capture at least20 genuine Go JSON outputs with command/toolchain/provenance. Synthetic handwritten event tests remain useful but do not count as genuine captured-output goldens. Distinguish unmodified public repo output from controlled owned fixtures/fault injection.
- [x] Select and record10 reputable public Go module repositories with immutable commit SHAs. Run only controlled manual validation in the existing approved self-use boundary; no stranger submission or public listener.
- [x] Check reports against outputs: cases/coverage/failed source locations/incomplete/truncation; preserve machine-readable results and hashes. Do not pass an unavailable repo/unsupported toolchain or count reruns as distinct repos.
- [x] At least20 captured-output regression cases and10 real repo reports must be verified before exiting PUBLIC-1. If an environment blocks a required run, record NOT RUN and keep this stage in progress.

## Task 4: Stage review and checkpoint

- [x] `go test -p=1 -count=1 ./...`, `go vet ./...`, `go build ./...`, gofmt checks; required affected Linux/container checks and browser QA.
- [x] Independent spec review, then quality review of final diff and evidence. Do not hide AUD-001/002/003 behind report success; they remain future release blockers.
- [ ] Update publicization-stage-report.md with exact PASS/FAIL/NOT RUN counts, corpus/repo evidence, measured diff, retries/blocks and next stage. Commit/push only the verified feature branch under existing repository authorization; do not merge/deploy public behavior or mark public opening approved.
