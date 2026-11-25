# RepoTest Progress Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the verified S3 browser/session and S5 deployment-validation gaps while preserving the frozen access and deployment boundaries.

**Architecture:** Reuse the current embedded Go frontend, SQLite session hashes, loopbackOnly guard, read-only LAN handler and shell operations checks. Work only in the existing isolated continuation copy.

**Tech Stack:** Existing Go/SQLite, plain HTML/JavaScript, POSIX shell and Python standard library. No new dependencies.

## Task 1: S5 installation and repeatable verification

Files: .github/workflows/ci.yml, docs/deployment.md, scripts/verify-s5-operations.sh, scripts/verify-linux-acceptance.sh, scripts/invoke-linux-acceptance.ps1.

- [x] Reproduce shell syntax gap with two temporary scripts: first valid, second `if then`; `sh -n first second` exits 0, per-script loop exits nonzero. Save evidence under work/progress-repair-20261123.
- [x] Replace CI syntax command with `for script in scripts/*.sh; do sh -n "$script" || exit; done`.
- [x] Add concrete dry-run build/hash and separately authorized install steps for both binary files, dedicated account and real home, private directory ownership, unit/env installation and daemon-reload. Keep service startup and restore authorization explicit.
- [x] Add a focused shell entry that uses mktemp/trap fixtures only, runs existing backup tests and Python health regression, and checks valid preflight plus relative/symlink/private-mode/runner-gate failures. It must not deploy or change an actual database.
- [x] Invoke it from the Linux full acceptance entry. Archive deploy files and hash all transmitted Go/mod/sum/script/deploy artifacts; include source.sha256 and verify it on target before CRLF normalization.
- [x] Verify changed shell syntax, the two-script reproduction, PowerShell parsing, archive/manifest consistency and independent spec/code review. Target execution remains BLOCKED until SSH access works.

## Task 2: Production session revocation and LAN read-only browser flow

Files: internal/control/sessions.go, sessions_test.go, pairing.go, handler.go, web/lan.html, LAN regression tests, web/index.html.

- [x] Write failing integration tests using SQLite, a paired LAN cookie and existing loopbackRequest helper: local list/revoke are guarded; one revoked device receives 401 while another stays valid; no raw tokens are returned; illegal IDs and unavailable storage have explicit errors.
- [x] Implement active-session list and per-hash revocation with existing sessions table and loopbackOnly wrappers. Routes are GET /api/v1/sessions and DELETE /api/v1/sessions/{id}; LAN exposes neither.
- [x] Write failing LAN tests: GET / serves a pair/read-only page without CSRF meta, unknown path remains 404, unpaired APIs 401, paired task/report/log readable, all task/session writes 405, bad Host refused.
- [x] Embed a small same-origin pair/task-read page in the existing visual style. Keep every request read-only apart from existing POST /pair and escape untrusted values.
- [x] Add local session-list/revoke controls and active-task delete controls. Use a view revision and timer cleanup to prevent late polling/report replies after delete or a new preview.
- [x] Run focused and full Go regression plus actual browser interaction (or retain a clear limitation if the browser cannot run); independently review access policy before proceeding.

## Task 3: Reconcile progress and deliver a reviewed repair package

Files: README.md, docs/project-design.md, optimization-design.md, stage-report.md, the existing S5 plan.

- [x] Mark stale S3 design text as dated historical context; preserve historical PASS evidence and distinguish repaired implementation, Windows checks, target Linux verification and deployment acceptance.
- [x] Record numbered issues, actual regression results, diff/rework/blocker metrics, artifact hashes and recovery steps in the existing three documents. Do not create a synthetic S6.
- [x] Generate a new package from original baseline to the final copy, preserving the earlier immutable ZIP. Include full changed files, SHA-256 manifests, review/check evidence and exact merge/rollback instructions.
- [x] Recheck original F:/repotext baseline read-only and run `git apply --check --ignore-whitespace` for the final patch. No original write or deployment occurs under the current permissions.

## Task 4: 2026-11-24 preflight boundary repair

- [x] Preserve the 94-file session baseline and record ISSUE-021 before implementation.
- [x] Extend existing Linux S5 fixtures for dangling database links, ancestor/trailing-slash links and invalid ports; retain the supported IPv6 port 0 case.
- [x] Reject every database/work path symlink component and validate numeric loopback ports; keep private permissions.
- [x] Run affected shell syntax, actual Windows junction guard checks, endpoint before/after regression and source archive byte checks.
- [x] Run the complete new fixture and cumulative S4/S5 entry on the authorized target Linux. User ran r17 externally: exit0,164 PASS/0 SKIP/0 FAIL, complete S4/S5 markers and resources clean. The agent independently verified83 source bytes/manifest/tar and output. r16 config rejection is historical; agent-local SSH remains restricted.

## Task 5: 2026-11-24 original merge and transfer diagnostics

- [x] Verify external ZIP SHA, CRC/paths/24 changed bytes and all86 original baseline files; reject new-target conflicts.
- [x] Save86 immutable .snapshot files, apply24 exact files (8 new), and verify all94 merged source files.
- [x] Keep extracted recovery sources under _package and verify go list finds only5 root packages; record and correct the initial discovery failure (ISSUE-023).
- [x] Reproduce CopyTo failure losing child stderr/result (ISSUE-022); add3 local child-process regressions, then preserve logs/exit codes without changing SSH options.
- [x] Re-run Windows full Go (156 PASS/6 SKIP/0 FAIL), vet/build/format, health, shell syntax and PowerShell AST; independently review merge/driver/83-file acceptance archive.
- [x] Run real r16 entry and retain exit255/SSH config Permission denied as BLOCKED; update authoritative F: handoff and stage records. Actual deployment remains NOT RUN.

Execution is authorized by the user's latest instruction to review progress and continue completing the project. No additional design confirmation is needed for these existing requirements. Actual deployment, target restore and observation remain gated as before.
