# S4 Linux Execution Implementation Plan

**Goal:** Run a confirmed Go commit on the target Debian host and persist a real test report.

**Architecture:** Stream an archive of the private task workspace through Docker stdin. The test container has a read-only image, a bounded tmpfs workspace, no network, and no host bind mounts. Dependency fetching and task scheduling are separate steps governed by AC-008.

1. Add a Docker integration test in `internal/runner/offline_test.go`. A local Go fixture must execute in the sandbox. Verify that timeout removes the container and that the test sees only loopback and cannot write to the image. Run it with `REPOTEST_TEST_IMAGE` set to a local immutable Go image ID.
2. Implement `internal/runner/offline.go` using fixed Docker arguments, bounded output, streamed regular files, and explicit container cleanup. On cancellation, remove the container by its generated name. Exercise the isolated backend on `instabox-test-env` before wiring it into the service.
3. Add dependency fetching through a dedicated, bounded Go module proxy. Require fixed upstreams, validated public endpoint IPs, no redirects or direct fallback, and restricted reachability from the dependency container. Transfer the resulting module cache into the offline test step.
4. Connect confirmation, SQLite outbox consumption, report persistence and local report display. Check recent-result reuse against `finished_at`, and retain old reports until TTL or explicit deletion. Verify cancellation and restart cleanup on the target host.
5. Run CPU/PID pressure, disk hard-limit, dependency isolation, timeout, cancellation, restart, and HTTP end-to-end checks. Keep execution disabled in the service until AC-008 has evidence for every condition.

## 2026-11-21 Local implementation checkpoint

- Implemented bounded Go proxy forwarding, one validated storage redirect for module ZIPs, public-IP pinning for upstream TLS connections, per-response and aggregate byte caps, and path/method restrictions. The dependency container uses a Docker `--internal` network and can only reach the host proxy; the test container remains `--network=none`.
- Implemented private fixed-commit source extraction, module-cache transfer into the no-network test container, SQLite confirmation/outbox consumption, attempt-fenced state changes, report/log persistence, local task status/report UI, cancellation and startup recovery hooks.
- A Linux advisory lock in the current OS user’s private `~/.local/state/repotest` directory prevents a second enabled process from sweeping live resources. The OS user home lookup ignores `HOME`/`XDG_CACHE_HOME` overrides, and Docker cleanup labels are scoped to the UID.
- Local `go test ./... -count=1` passes on Windows/Go 1.26.3. Linux-only Docker tests are skipped on this host. SSH host-key discovery to the supplied endpoint was denied by the current network sandbox; the expected fingerprint is therefore unverified and no target-host command has run.
- AC-008 remains open: actual runner configuration and proxy networking have not run on Debian; CPU/PID/memory pressure, persistent disk quota, cancel, restart cleanup and full HTTP execution flow are NOT RUN. Keep execution closed.
