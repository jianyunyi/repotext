# RepoTest Deployment and Operations

This document describes a dry-runable deployment package. It does not authorize installation, service enablement, LAN exposure, or execution of untrusted repositories.

## Current target continuation (2026-11-25)

The user explicitly approved the reviewed [2026-11-25 target plan](superpowers/plans/2026-11-25-s5-target-deployment.md). Installation, fixed work storage, DB/WAL/SHM recovery drill and monitoring passed in r22; full Linux acceptance r23 passed (164 tests, no skips/failures). The target now has an active loopback-only service with runner/LAN disabled. Seven-day observation began at `2026-11-25T06:36:33Z`; its candidate end is `2026-12-02T06:36:33Z`. Installation PASS and an initial archived sample do not accept that future observation window.

The deployed files are preserved byte-for-byte in `deploy/var-lib-repotest-work.mount`, `deploy/repotest-work.conf`, `deploy/repotest-health.service`, `deploy/repotest-health.timer`, `deploy/repotest-health-export.service`, `deploy/repotest-health-export.timer` and `scripts/export-health.sh`. The work drop-in is installed as `/etc/systemd/system/repotest.service.d/work-mount.conf`; the exporter is installed executable as `/usr/local/libexec/repotest-export-health`. Five-minute health sampling uses only the health unit journal, including state/storage fields even after probe failure. Hourly root-private JSON exports advance an adjacent since/until checkpoint only after valid export under a nonblocking `flock`; inclusive boundaries are deduplicated by `__CURSOR`. This automates the approved observation archiving, with a shorter window than the original daily procedure. Account for missing samples, rotation and incidents at final acceptance.

Actual installation/recovery evidence is in private `/var/backups/repotest-deployment.bMqjzCah`; the stopped-service original DB/WAL/SHM set is in `/var/lib/repotest/restore-before.AFxt6ttl`. Observation files/checkpoints live under root-owned 0700 `/var/backups/repotest`. These directories contain no material intended for Git. Use `systemctl list-timers repotest-health.timer repotest-health-export.timer` and only the observer-unit journal for monitoring; the main service journal contains pairing codes and must be kept private/redacted.

The generic installation excerpt below does not create a fixed-capacity work mount. Provision the approved persistent work filesystem and install its mount dependency/startup check before starting the service. The target plan recommends a new preallocated 2 GiB ext4 image, `mkfs.ext4 -E nodiscard` and allocated-block verification, a dedicated mount unit and `mountpoint` ExecStartPre; fail if any target object already exists. Startup needs bounded health readiness, and restore must demonstrate identifiable state rather than only an empty database. Default runner/LAN remain disabled; repeat hard-quota pressure and HTTP cleanup against the new persistent mount before any runner activation. The host filesystem's observed ext4 type is not proof of the hosting platform's lifecycle retention policy.

## Layout

- `deploy/repotest.service` is a systemd template for a dedicated `repotest` user.
- `deploy/repotest.env.example` keeps the service on loopback with execution disabled.
- `scripts/check-deployment.sh` validates paths, permissions, bind addresses, and runner gates.
- `cmd/repotest-backup` creates an integrity-checked SQLite backup while the service is stopped.
- `scripts/check-health.sh` is the liveness probe used by operators and CI.

The work directory must be a private directory on a fixed-size filesystem. The S4 target acceptance used a temporary 2 GiB ext4 loopback filesystem and verified that a 2.25 GiB write fails. A normal directory on the root filesystem is not equivalent.

## Prepare and verify (no installation)

Run from the source package directory on Linux. Build both commands into a temporary directory and record the exact artifacts for review; keep that directory until the authorized installation or discard it afterwards:

```sh
set -eu
build_dir=$(mktemp -d /tmp/repotest-build.XXXXXXXX)
go build -o "$build_dir/repotest" ./cmd/repotest
go build -o "$build_dir/repotest-backup" ./cmd/repotest-backup
sha256sum "$build_dir/repotest" "$build_dir/repotest-backup"
sh scripts/verify-s5-operations.sh
printf 'BUILD_DIR=%s\n' "$build_dir"
```

The S5 check uses only temporary fixtures. It runs the backup regressions, actual loopback HTTP health boundaries, and private-path/default-off preflight success and failure cases. `S5_SYSTEMD_CONTRACT_STATIC_PASS` only checks the supplied unit/environment text; it does not prove installation or service recovery. The complete `verify-linux-acceptance.sh` entry point includes this S5 check alongside the existing S4 regression.

After full target acceptance, `sh scripts/prepare-s5-deployment.sh` collects PID1/systemd, effective UID/`sudo -n true` probe, actual `sudo -n -l` rules, account/group, listener and existing-path facts, then builds these two binaries and preserves the binaries/unit/environment/probes in a private `/tmp/repotest-s5-prepared.XXXXXXXX` directory. The probe alone does not establish permissions for useradd/install/runuser/systemctl; the script explicitly reports `INSTALL_COMMAND_PERMISSIONS=UNVERIFIED`. It prints the prepared path and `artifacts.sha256`; source transfer is verified by the Windows driver before script CRLF normalization. `S5_PREPARATION_PASS` means facts and artifacts were prepared, not that systemd is available or installation is approved. Inspect every reported prerequisite before selecting the actual installation or upgrade steps. This command does not install a service, edit accounts, read environment contents or touch the actual database. Its private prepared directory is intentionally retained for the reviewed next step; keep its exact path until installation, or remove only that known directory once no longer needed.

For the authorized `instabox-test-env` target from Windows:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/invoke-linux-acceptance.ps1 -RemoteScript scripts/prepare-s5-deployment.sh -EvidenceName s5-deploy-preparation-20261124-r18
```

On Windows, `scripts/invoke-linux-acceptance.ps1 -PrepareOnly -EvidenceName <new-name>` prepares the archive and manifest without connecting to SSH. The archive includes Go sources, module files, scripts, deployment files and `source.sha256`. Normal target execution verifies these bytes with `sha256sum -c` before normalizing CRLF and running checks.

## Install and start (separate explicit authorization required)

These steps change the target host and start a service; preparation or a static review does not authorize them. Run as root from the reviewed source package directory only after approving the target, binary hashes and steps. Use the `build_dir` printed above and verify its binaries against the recorded hashes before installing.

```sh
set -eu
test -x "$build_dir/repotest"
test -x "$build_dir/repotest-backup"
# For a new installation. If the account already exists, verify its identity
# and home before proceeding; do not silently modify an existing account.
if ! getent passwd repotest >/dev/null; then
    useradd --system --user-group --home-dir /var/lib/repotest --no-create-home --shell /usr/sbin/nologin repotest
fi
test "$(getent passwd repotest | cut -d: -f6)" = /var/lib/repotest
test "$(id -gn repotest)" = repotest
install -d -o repotest -g repotest -m 0700 /var/lib/repotest /var/lib/repotest/work
install -d -o root -g root -m 0700 /etc/repotest /var/backups/repotest
install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "$build_dir/repotest" /usr/local/libexec/repotest
install -o root -g root -m 0755 "$build_dir/repotest-backup" /usr/local/libexec/repotest-backup
install -o root -g root -m 0600 deploy/repotest.env.example /etc/repotest/repotest.env
install -o root -g root -m 0644 deploy/repotest.service /etc/systemd/system/repotest.service
# Root opens the script so a private source directory does not prevent the
# service user from running the preflight against the absolute target paths.
runuser -u repotest -- env REPOTEST_DB=/var/lib/repotest/repotest.db REPOTEST_WORK_DIR=/var/lib/repotest/work REPOTEST_ADDR=127.0.0.1:8080 REPOTEST_LAN_ADDR= REPOTEST_RUNNER_ENABLED=0 REPOTEST_AC008_ACCEPTED=0 sh < scripts/check-deployment.sh
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/repotest.service
systemctl enable --now repotest.service
sh scripts/check-health.sh http://127.0.0.1:8080/healthz
```

The default environment keeps `REPOTEST_RUNNER_ENABLED=0` and leaves `REPOTEST_LAN_ADDR` empty. A new service account uses `/var/lib/repotest` as its real, owned home so its future runner lock can live below `.local/state/repotest`; it is inside the unit's writable path and outside the directories hidden by `ProtectHome=true`. This does not authorize runner enablement. The script above is for a new installation; upgrades must separately preserve the current unit, environment, binary and database before replacement.

The unit joins the Docker group because the current runner uses the Docker socket. That is a rootful-daemon trust boundary; it does not provide a strong boundary against a compromised host or Docker daemon.

## Backup and restore

Stop the service before backup or restore so the database and WAL are quiescent. Create `/var/backups/repotest` as a real directory with mode `0700` before the first backup:

```sh
systemctl stop repotest.service
/usr/local/libexec/repotest-backup -db /var/lib/repotest/repotest.db -out /var/backups/repotest/repotest-$(date -u +%Y%m%dT%H%M%SZ).db
```

The command opens the source read-only for `PRAGMA integrity_check`, then uses an isolated SQLite connection for `VACUUM INTO` (the source file is not modified). It reserves the destination exclusively with mode `0600`, refuses overwrite even between concurrent invocations, and runs `PRAGMA integrity_check` on the output. Failed calls only remove their own reserved output.

Restore must preserve the current database together with any `-wal` and `-shm` files. A crash or forced stop can leave a WAL even after the service stops; leaving that WAL next to a replacement main database can replay stale writes. Run the following from the source package directory as root, with an explicitly selected backup, and ensure no manually started process is using the database:

```sh
set -eu
systemctl stop repotest.service
test "$(systemctl show -p MainPID --value repotest.service)" = 0
recovery_dir=$(mktemp -d /var/lib/repotest/restore-before.XXXXXXXX)
chmod 0700 "$recovery_dir"
for suffix in '' '-wal' '-shm'; do
    current="/var/lib/repotest/repotest.db$suffix"
    if test -e "$current"; then
        mv -- "$current" "$recovery_dir/"
    fi
done
/usr/local/libexec/repotest-backup -db /var/backups/repotest/selected.db -out /var/lib/repotest/repotest.db
chown repotest:repotest /var/lib/repotest/repotest.db
chmod 0600 /var/lib/repotest/repotest.db
REPOTEST_DB=/var/lib/repotest/repotest.db REPOTEST_WORK_DIR=/var/lib/repotest/work sh scripts/check-deployment.sh
systemctl start repotest.service
sh scripts/check-health.sh http://127.0.0.1:8080/healthz
```

The backup command verifies both source and restored output before startup. Any failed step blocks subsequent steps; keep the service stopped and retain `recovery_dir`. To undo a failed restore, preserve the replacement database and its sidecars separately, then restore the complete original set from `recovery_dir` while stopped, confirm ownership and integrity, and rerun the preflight. The r22 actual target drill exercised this procedure with a real preserved DB/WAL/SHM set and a synthetic record; instructions alone remain insufficient for a new target.

## Monitoring and rollback

Run `scripts/check-health.sh http://127.0.0.1:8080/healthz` from systemd timer, cron, or an external monitor. A non-zero result is an alert; it does not restart the service by itself. `systemctl status`, the journal, and the persisted SQLite database are the primary diagnostics.

The probe only accepts explicit loopback URLs, requires HTTP 200, and checks the JSON status field. It prints `HEALTH_PASS` on success and exits non-zero with `HEALTH_FAIL` for connection, timeout, address, or response errors.

CI runs Go test, vet, and build on Windows and Ubuntu. The Ubuntu job also checks every shell script and runs the health probe HTTP boundary regression. CI keeps both runner gates at `0`; it does not pull runner images, expose LAN listeners, deploy, or push artifacts.

Rollback is: stop the service, restore the previous binary and verified database backup, run the preflight, and start the service. Do not use `git reset`, force replacement, or an unverified database copy as a rollback step.

## Acceptance boundary

The smoke set is Windows Go tests, vet and build plus the current `scripts/verify-linux-acceptance.sh` entry point, which includes S4 isolation/quota/HTTP cleanup and S5 backup/health/preflight. Historical r10/r14 results do not verify later repairs. Preflight now rejects symlinks in any database/work path component, including dangling database links and trailing-slash paths, and validates loopback ports in the service's supported 0–65535 range. Port 0 is an ephemeral development listener; the default deployment and health probe use the explicit port 8080.

Actual target installation and the service restore drill passed in r22. Seven-day observation remains incomplete S5 evidence; stability is not accepted before that observation. Public exposure and unfamiliar-user execution require a separate scope decision. Default runner and LAN settings remain disabled.
