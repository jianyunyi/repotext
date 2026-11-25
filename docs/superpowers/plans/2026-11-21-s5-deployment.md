# S5 Deployment and Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a reproducible, reversible deployment and operations package for RepoTest, with backups, health checks, cross-platform CI, and a full regression entry point.

**Architecture:** Keep the service as one Go binary. A systemd unit and environment template define a loopback-only, runner-disabled default; enabling the runner requires the already verified immutable image and explicit AC-008 acceptance. A small Go backup command uses SQLite `VACUUM INTO` while the service remains stopped, and shell preflight/health scripts validate permissions, paths, and liveness without deploying.

**Tech Stack:** Go 1.23+, modernc.org/sqlite already in the module, systemd, POSIX shell, GitHub Actions.

---

### Task 1: Deployment contract and preflight

**Files:**
- Create: `deploy/repotest.service`
- Create: `deploy/repotest.env.example`
- Create: `scripts/check-deployment.sh`
- Create: `docs/deployment.md`

- [x] **Step 1: Write the preflight checks**

  The script must require an absolute private database path and work directory, reject symlinks and group/world writable directories, require a 64-hex image digest when runner mode is enabled, reject non-loopback `REPOTEST_ADDR`, and report `runner_disabled` when either gate is unset.

- [x] **Step 2: Run the preflight against a temporary valid fixture**

  Run `sh scripts/check-deployment.sh` with a temporary 0700 database parent, work directory, loopback address, and runner disabled. Expected: exit 0 and `DEPLOYMENT_PREFLIGHT_PASS`.

- [x] **Step 3: Run failure cases**

  Run with a relative work path, a symlinked work path, and `REPOTEST_RUNNER_ENABLED=1` without `REPOTEST_AC008_ACCEPTED=1`. Expected: non-zero with a specific reason for each case.

- [x] **Step 4: Add the systemd/env templates and operations document**

  The unit must use a dedicated user, `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, explicit `ReadWritePaths`, `Restart=on-failure`, and loopback defaults. The document must include install, stop-before-backup, restore, rollback, health check, and rootful Docker limitations.

- [x] **Step 5: Re-run the preflight and shell syntax checks**

  Run `sh -n scripts/check-deployment.sh scripts/verify-linux-acceptance.sh scripts/verify-disk-quota.sh scripts/verify-http-runner.sh`. Expected: exit 0.

### Task 2: Consistent SQLite backup and restore validation

**Files:**
- Create: `cmd/repotest-backup/main.go`
- Create: `cmd/repotest-backup/main_test.go`
- Modify: `docs/deployment.md`

- [x] **Step 1: Write backup command tests**

  Cover a valid database producing a readable backup, refusal of a destination inside the source database directory when it is a symlink, and `PRAGMA integrity_check` returning `ok` for the output.

- [x] **Step 2: Run the focused tests and observe the missing command failure**

  Run `go test ./cmd/repotest-backup -count=1`. Expected: compile failure until the command exists.

- [x] **Step 3: Implement the minimal command**

  Accept `-db` and `-out`, require absolute regular paths, open the source read-only, run `VACUUM INTO` with a safely quoted SQL literal, close both handles, reopen the output read-only, and run `PRAGMA integrity_check`. Create the output with mode 0600 and refuse overwrite.

- [x] **Step 4: Run focused tests and a manual backup/restore dry-run**

  Run `go test ./cmd/repotest-backup -count=1` and invoke the binary against a temporary RepoTest database. Expected: PASS and an integrity-checked backup; no service is stopped or replaced.

### Task 3: Monitoring and CI

**Files:**
- Create: `scripts/check-health.sh`
- Create: `.github/workflows/ci.yml`
- Modify: `docs/deployment.md`

- [x] **Step 1: Add health probe behavior**

  The probe must require an explicit URL, use curl with a timeout, require HTTP 200, and print a stable `HEALTH_PASS`/`HEALTH_FAIL` marker.

- [x] **Step 2: Add cross-platform CI**

  The workflow must run Windows and Ubuntu Go test/vet/build jobs, keep the runner disabled, and run shell syntax checks on Ubuntu. It must not push, deploy, or expose LAN listeners.

- [x] **Step 3: Exercise the probe locally**

  Start the existing loopback service with runner disabled, run the probe for `/healthz`, and run it against an unused port. Expected: 0 with `HEALTH_PASS`, then non-zero with `HEALTH_FAIL`.

### Task 4: Full regression and S5 report

**Files:**
- Modify: `docs/project-design.md`
- Modify: `docs/stage-report.md`
- Modify: `docs/optimization-design.md`

- [x] **Step 1: Run the Windows smoke set**

  Run `gofmt -l cmd internal`, `go test -p=1 -count=1 ./...`, `go vet ./...`, and `go build ./...` with an isolated `GOCACHE`. Expected: all exit 0.

- [x] **Step 2: Run the Linux r12 acceptance entry point**

  Run `scripts/invoke-linux-acceptance.ps1` and record the evidence directory and archive hash. Expected: S4 smoke remains PASS and the deployment additions do not enable execution by default.

- [x] **Step 3: Update the state blocks and report**

  Record AC-009 PASS/FAIL per check, preserve the rootful daemon risk, record any NOT RUN item, and leave the next step explicit. Do not claim production stability or perform deployment.

### 2026-11-23 continuation: S5 repair and handoff

The original plan above is historical. Current implementation first verifies the source read-only, then reserves an empty 0600 destination exclusively before VACUUM INTO. No source writes are authorized during backup.

- [x] Reproduce and repair post-validation destination creation; verify the competitor's completed bytes survive and eight concurrent callers produce exactly one complete backup. Files: cmd/repotest-backup/main.go and main_test.go. Commands: go test ./cmd/repotest-backup -count=1; go vet ./cmd/repotest-backup.
- [x] Reproduce HTTP201 acceptance and userinfo bypass; require numeric loopback authority and HTTP200. Files: scripts/check-health.sh, scripts/test-check-health.py, .github/workflows/ci.yml. Command: python3 scripts/test-check-health.py (Windows uses --shell with the Git bash path). Expected: four PASS cases with invalid authorities rejected before a request.
- [x] Preserve stopped-service DB/WAL/SHM, restore through the integrity-checked backup command, assign repotest ownership, preflight, then start. File: docs/deployment.md. Independently review these instructions; actual target drill remains below.
- [x] Run Windows go test -p=1 -count=1 ./..., go vet ./..., go build ./..., gofmt -l cmd internal and each shell script syntax check. Record exit codes and review evidence.
- [ ] Merge the repair copy into F:/repotext only after comparing baseline hashes; this session cannot write that project.
- [ ] Run powershell -NoProfile -ExecutionPolicy Bypass -File scripts/invoke-linux-acceptance.ps1 -EvidenceName s5-linux-20261123-r15 in the authorized project workspace. Expected: complete S4 regression and resource cleanup, plus focused S5 health/backup verification. SSH configuration access is BLOCKED in this session.
- [ ] Obtain authorization for the existing target's systemd installation and actual restore drill, then run the documented steps; retain the seven-day observation requirement.

### Current continuation: progress review and gap repair

The preceding completed entries record their dated implementation checks, not complete user-flow or deployment acceptance. The 2026-11-23 progress review found and repaired the S3 session/LAN/active-delete browser gaps and S5 install/CI/target-entry gaps; see 2026-11-23-progress-repair.md and the latest stage-report.md section.

The current Linux entry now calls verify-s5-operations.sh. Invoke with a fresh evidence directory after the new unified repair package is merged; -PrepareOnly builds and hashes the transmission archive without SSH. The target still must run the complete regression. Actual systemd installation, service restore and seven-day observation remain pending.
