# S5 Target Deployment and Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to execute the authorized tasks. Checkboxes distinguish completed preparation from pending host changes.

**Goal:** Install the reviewed RepoTest build on the existing `instabox-test-env`, demonstrate service recovery, and collect seven days of operational evidence.

**Architecture:** Reuse the current Go binaries, dedicated service account, systemd template and backup command. Add a dedicated 2 GiB ext4 work filesystem and a service mount dependency. Keep both runner gates at `0` and LAN unset throughout this deployment.

**Tech Stack:** Existing Go/SQLite service, systemd, ext4 loop mount, POSIX shell and installed Python SQLite support. No new dependencies.

## Current status and authorization

2026-11-25: user explicitly approved this target installation, 2 GiB work filesystem, restore drill and health observation. Installation, mount/capacity checks, the restore drill and monitor setup passed in r22; complete Linux r23 passed. Seven-day coverage is in progress and not accepted. Source baseline: `a7fe305a33fe554b5975f2c86e62e440fecf1faa`. The latest user reply “批准” explicitly authorizes the concrete host changes below on instabox-test-env. Continue without requesting the same authorization again; root/SSH operations still follow environment approval rules. Actual reboot, public exposure and runner/LAN activation remain outside this scope. Previous preparation-only authorization is historical.

Target facts from `work/s5-deploy-preparation-20261125-r20/`: PID1 systemd, state running, Docker active, operator UID 1000 and verified noninteractive sudo UID 0; no existing repotest account/group, service, deployment directories or listener on 8080. Mount tools are installed at the paths used below. `/var/lib` is on the host's ext4 root filesystem; `df -P` reports 4,240,024 KiB available (about 4.04 GiB). Allocate 2 GiB only after rechecking capacity, leaving about 2 GiB for the host. This is a recommendation based on the previously tested S4 capacity, not a new user requirement. Confirm the volume will persist across target lifecycle/reboot; the inspection alone cannot guarantee the hosting platform's retention policy.

Prepared directory: `/tmp/repotest-s5-prepared.oKH4ntJ9` (private, temporary; may disappear at reboot). Reviewed SHA-256 values:

```text
0bdadb52136b32cdcb1fb768a34de53fcdd112d7407e8b10ad5998eac7e89301  repotest
795bbe9ecee9532c0844c1a3673f64e865006025c7464dbb03cacc9c4f8fdfef  repotest-backup
9f9d26f95a6a48092ad7050f020ed649887273eda78c37c057f40750d81b016c  repotest.service
e24473b17a88788954bce93c9ca3aa37a19bf34fc6bed3d7856551ad3c5d60bb  repotest.env.example
4e251db3e3e8831d7da9bd802307d724b8795843a7719794890ae948be96dc7b  check-health.sh
823a8c573f367e89c847d75d269c458d79f0ccba0cd07dc054e4f13e472823b2  check-deployment.sh
```

Additional read-only storage check: `/var/lib` is on root `/dev/vda`, while `/data/home` is on the separate ext4 `/dev/vdb` mounted at `/data`, with 16,509,224 KiB free (about 15.7 GiB). The complete Linux smoke's second 2 GiB temporary quota image lives under `/data/home`, so it does not consume the remaining root space. Recheck these distinct mounts and available space before the seven-day exit smoke; this is observed capacity, not a platform durability guarantee. New probe/mount/drop-in/health unit paths were also absent.

## Actual execution checkpoint (2026-11-25)

User approval and subsequent continue are recorded in work/s5-install-20261125-r22/authorization.json. Precheck, installation, persistent capacity, exact guard in an isolated mount namespace, host dependency remount, actual synthetic-session/WAL recovery and monitor setup all exited 0. Windows smoke passed; full Linux r23 has 164 PASS / 0 SKIP / 0 FAIL and final resources clean. Seven deployed config files are stored byte-for-byte in deploy/ and scripts/export-health.sh with matching target SHA-256.

Observation starts 2026-11-25T06:36:33Z and its candidate seven-day end is 2026-12-02T06:36:33Z. The initial complete sample is archived; actual seven-day coverage remains NOT RUN. The approved daily adjacent-window archive is automated hourly by root-owned repotest-health-export.service/timer and scripts/export-health.sh, reducing dependence on manual collection without changing the scope or health interval. Checkpoints advance only after successful valid export, with flock overlap protection. Main app journal and all DB/archives remain private. Completed preparation/installation/drill/setup steps are checked below. Continuous seven-day coverage and its exit checks remain unchecked; runtime evidence is authoritative in stage-report.md and the recorded result files.

## Frozen acceptance and scope

- AC-009/install: installed binary hashes match the reviewed values; account/home and DB/work permissions match the contract; only `127.0.0.1:8080` listens; runner and LAN remain disabled; actual unit and drop-in verification succeeds.
- AC-008/work and ISSUE-025: image file is exactly 2,147,483,648 bytes and fully allocated, mount type is ext4 with nodev/nosuid/noexec, mounted work directory is repotest-owned 0700. Startup requires the mount. The 2 GiB image includes filesystem metadata; available application space is smaller. Before enabling any runner, repeat the existing hard-quota pressure and HTTP/cleanup checks on this persistent work mount. Runner stays disabled in this plan.
- AC-009/restore: stopped-service backup passes integrity, preserves a recognizable synthetic session record, restores its pre-change state, cleans that record, and starts healthy. Preserve the current DB/WAL/SHM as a complete recovery set before replacement. Empty-database integrity alone is insufficient.
- AC-009/monitor: seven actual consecutive days of timestamped liveness, service state, unexpected restarts and work/root filesystem usage. Count missing checks as missing evidence, and record incidents. Default-off service observation does not establish runner stability.
- Smoke: current Windows Go test/vet/build; complete Linux acceptance at stage exit, with S4 isolation/quota/HTTP cleanup and S5 backup/health/path boundaries. Fixture results do not replace real installation/restore checks.
- Deliberate simplification: a fixed 2 GiB work image reuses the tested storage approach for this single host. Database and backup storage remain on the root filesystem and require separate monitoring; add a dedicated data volume if actual growth threatens host headroom. No public exposure, LAN activation, runner enablement, host reboot or changes to unrelated services.

## Task 1: Preserve preparation and recheck prerequisites

**Files:** evidence under `work/s5-deploy-preparation-20261125-r20/`; existing `docs/deployment.md` remains the generic operations reference.

- [x] Transfer and verify all 84 source files before newline normalization; prepare six artifacts and verify their hashes.
- [x] Verify target systemd, root privilege, paths, 8080 availability and storage/tool facts.
- [x] After approval, reconnect to the same target; recheck identity, free space, paths and account absence. Fail on any existing target file, directory, symlink, account, unit, image or 8080 listener; an existing installation requires an upgrade plan.
- [x] As root, create a private evidence directory with `mktemp -d /var/backups/repotest-deployment.XXXXXXXX`. Save the approved plan, artifact hashes, before facts and new unit contents there. Verify each prepared artifact against the literal hashes above, not only its neighboring mutable manifest. If the temporary prepared directory vanished or bytes changed, rebuild and re-review its new hashes.

## Task 2: New installation with persistent work storage

**Files changed on target:** `/usr/local/libexec/repotest`, `/usr/local/libexec/repotest-backup`, `/usr/local/libexec/repotest-check-health`, `/etc/repotest/repotest.env`, `/etc/systemd/system/repotest.service`, `/etc/systemd/system/repotest.service.d/work-mount.conf`, `/etc/systemd/system/var-lib-repotest-work.mount`, `/var/lib/repotest-work.ext4`; new repotest account/home/work and private backup directory. Task 4 also adds `/etc/systemd/system/repotest-health.service` and `repotest-health.timer`. Reuse the six reviewed artifacts. No source-code changes are required.

- [x] Run the following only after prerequisite checks and explicit installation authorization. Root shell variables must retain the reviewed `build_dir` and private `evidence_dir` from Task 1.

```sh
set -eu
umask 077
build_dir=/tmp/repotest-s5-prepared.oKH4ntJ9
test "$(id -u)" = 0
for path in /var/lib/repotest /var/lib/repotest-work.ext4 /etc/repotest /var/backups/repotest /usr/local/libexec/repotest /usr/local/libexec/repotest-backup /usr/local/libexec/repotest-check-health /etc/systemd/system/repotest.service /etc/systemd/system/repotest.service.d /etc/systemd/system/var-lib-repotest-work.mount /etc/systemd/system/repotest-health.service /etc/systemd/system/repotest-health.timer; do
    test ! -e "$path" && test ! -L "$path"
done
if getent passwd repotest >/dev/null || getent group repotest >/dev/null; then exit 1; fi
useradd --system --user-group --home-dir /var/lib/repotest --no-create-home --shell /usr/sbin/nologin repotest
install -d -o repotest -g repotest -m 0700 /var/lib/repotest /var/lib/repotest/work
install -d -o root -g root -m 0700 /etc/repotest /var/backups/repotest
# Existing /usr/local/libexec is root-owned 0755; do not change it.
install -o root -g root -m 0755 "$build_dir/repotest" /usr/local/libexec/repotest
install -o root -g root -m 0755 "$build_dir/repotest-backup" /usr/local/libexec/repotest-backup
install -o root -g root -m 0600 "$build_dir/repotest.env.example" /etc/repotest/repotest.env
install -o root -g root -m 0644 "$build_dir/repotest.service" /etc/systemd/system/repotest.service
install -o root -g root -m 0755 "$build_dir/check-health.sh" /usr/local/libexec/repotest-check-health
# Reserve a new image; never truncate or format an existing object.
(set -C; : > /var/lib/repotest-work.ext4)
chmod 0600 /var/lib/repotest-work.ext4
fallocate -l 2147483648 /var/lib/repotest-work.ext4
test "$(stat -c %s /var/lib/repotest-work.ext4)" = 2147483648
mkfs.ext4 -E nodiscard -q -m 0 /var/lib/repotest-work.ext4
image_blocks=$(stat -c %b /var/lib/repotest-work.ext4)
test "$((image_blocks * 512))" -ge 2147483648
cat > /etc/systemd/system/var-lib-repotest-work.mount <<'UNIT'
[Unit]
Description=RepoTest fixed-capacity work filesystem
Before=repotest.service
[Mount]
What=/var/lib/repotest-work.ext4
Where=/var/lib/repotest/work
Type=ext4
Options=loop,nodev,nosuid,noexec
[Install]
WantedBy=local-fs.target
UNIT
install -d -o root -g root -m 0755 /etc/systemd/system/repotest.service.d
cat > /etc/systemd/system/repotest.service.d/work-mount.conf <<'UNIT'
[Unit]
RequiresMountsFor=/var/lib/repotest/work
[Service]
ExecStartPre=/usr/bin/mountpoint -q /var/lib/repotest/work
UNIT
chmod 0644 /etc/systemd/system/var-lib-repotest-work.mount /etc/systemd/system/repotest.service.d/work-mount.conf
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/repotest.service /etc/systemd/system/var-lib-repotest-work.mount
systemctl enable --now var-lib-repotest-work.mount
mountpoint -q /var/lib/repotest/work
chown repotest:repotest /var/lib/repotest/work
chmod 0700 /var/lib/repotest/work
findmnt -T /var/lib/repotest/work
runuser -u repotest -- env REPOTEST_DB=/var/lib/repotest/repotest.db REPOTEST_WORK_DIR=/var/lib/repotest/work REPOTEST_ADDR=127.0.0.1:8080 REPOTEST_LAN_ADDR= REPOTEST_RUNNER_ENABLED=0 REPOTEST_AC008_ACCEPTED=0 sh < "$build_dir/check-deployment.sh"
systemctl enable --now repotest.service
```

- [x] Validate installed hashes, account identity, file modes, loopback-only listener and mount properties. For each start/restart use this bounded readiness check. The bound is an operational timeout for this deployment, not a performance promise:

```sh
ready=0
for attempt in $(seq 1 30); do
    if sh /usr/local/libexec/repotest-check-health http://127.0.0.1:8080/healthz; then ready=1; break; fi
    sleep 1
done
if test "$ready" != 1; then
    systemctl stop repotest.service
    systemctl show repotest.service -p ActiveState -p SubState -p Result -p NRestarts
    exit 1
fi
```

Each health request also has the existing curl timeout, so the total bound includes request duration. Keep health failures during startup separately from final success. Never print raw journal output: startup logs contain a live five-minute pairing code. Store raw diagnostics only in root-private evidence; share a redacted copy excluding pairing/session tokens.

- [x] Stop only RepoTest, stop its own mount unit, confirm the lower work directory is not a mount, then start RepoTest and verify systemd restores the mount dependency. In a private unit namespace or other isolated fixture, verify a failing mountpoint ExecStartPre rejects startup; do not hide/remove production data to manufacture failure. Real mount-failure behavior remains NOT RUN until demonstrated.

## Task 3: Actual stopped-service backup and restore drill

**Files:** only this new service's DB/WAL/SHM and private `/var/backups/repotest` evidence; no other database is touched.

- [x] Stop RepoTest and require `systemctl show -p MainPID --value repotest.service` to equal `0`; verify no manual process is using this DB. Save a baseline backup before introducing drill data.
- [x] While stopped, use installed Python `sqlite3` to insert a synthetic session into the real new DB: `token_hash=secrets.token_bytes(32)`, `expires_at=int(time.time())+3600`, `revoked=0`. Save only its hex hash and expected fields in a root-private 0600 drill file. This is an unreachable synthetic hash, not a usable client credential or claimed real user state. Commit and close the SQLite connection.
- [x] Run `/usr/local/libexec/repotest-backup -db /var/lib/repotest/repotest.db -out /var/backups/repotest/drill-selected.db`; require `BACKUP_INTEGRITY_PASS`. Then, while still stopped, change exactly that session's revoked value to `1` and verify it changed. This gives the restore a distinguishable pre-change value.
- [x] Create a root-private `mktemp -d /var/lib/repotest/restore-before.XXXXXXXX` and move the complete current main DB/WAL/SHM set into it. Preserve that exact directory and do not overwrite it on retry. Restore with the backup command from `drill-selected.db` into the now-absent DB path; require integrity PASS, then chown repotest:repotest and chmod 0600.
- [x] Query the restored synthetic session and require `revoked=0` and the saved expiry; remove only this drill row, commit, close and require `PRAGMA integrity_check` to return `ok`. Rerun preflight as repotest, then start and run bounded readiness. Record before/change/after values, both integrity markers and health result. Synthetic state is not evidence of task/report recovery; those schema paths retain their test coverage and require a richer drill if actual user data later exists.
- On any restore failure, keep RepoTest stopped. Preserve any partial replacement DB/WAL/SHM in a separate new private directory; put the full original set back from `restore-before`, verify integrity, ownership and preflight before restarting. Never copy only the main DB over sidecars.

## Task 4: Seven-day observation and stage exit

**Files:** root-private `/var/backups/repotest/observation-*` evidence and project stage report; no raw credential-bearing journal in Git.

- [x] Start observation only after installation and drill pass. First demonstrate health PASS and an isolated refused-port FAIL; refuse any preexisting timer/service path. Install these root-owned 0644 units, run daemon-reload and `systemd-analyze verify` on them, then `systemctl enable --now repotest-health.timer` and record `systemctl list-timers repotest-health.timer` and the actual observation start timestamp:

```sh
cat > /etc/systemd/system/repotest-health.service <<'UNIT'
[Unit]
Description=RepoTest health and storage observation
[Service]
Type=oneshot
User=repotest
Group=repotest
ExecStart=/bin/sh /usr/local/libexec/repotest-check-health http://127.0.0.1:8080/healthz
ExecStopPost=/usr/bin/systemctl show repotest.service -p ActiveState -p SubState -p NRestarts
ExecStopPost=/usr/bin/df -P /var/lib/repotest /var/lib/repotest/work
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
UNIT
cat > /etc/systemd/system/repotest-health.timer <<'UNIT'
[Unit]
Description=Check RepoTest every five minutes
[Timer]
OnCalendar=*:0/5
AccuracySec=1s
Persistent=true
Unit=repotest-health.service
[Install]
WantedBy=timers.target
UNIT
chmod 0644 /etc/systemd/system/repotest-health.service /etc/systemd/system/repotest-health.timer
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/repotest-health.service /etc/systemd/system/repotest-health.timer
systemctl start repotest-health.service
systemctl enable --now repotest-health.timer
systemctl list-timers repotest-health.timer
date -u +%Y-%m-%dT%H:%M:%SZ > /var/backups/repotest/observation-start.txt
cp /var/backups/repotest/observation-start.txt /var/backups/repotest/observation-last-export.txt
```

Only this observation unit's output contains the health marker and nonsecret service/storage fields. Export adjacent, complete intervals to private archives daily; an arbitrary `--since today` export can lose the previous evening. Refuse existing start/checkpoint files before beginning observation. The following advances the checkpoint only after successful export; preserve failed output and retry the original window. Deduplicate inclusive boundary entries using each JSON journal entry's `__CURSOR` when counting samples.

```sh
set -eu
umask 077
since=$(cat /var/backups/repotest/observation-last-export.txt)
until=$(date -u +%Y-%m-%dT%H:%M:%SZ)
archive=$(mktemp /var/backups/repotest/observation.XXXXXXXX.json)
if journalctl -u repotest-health.service --since "$since" --until "$until" --no-pager -o json > "$archive"; then
    printf '%s\n' "$until" > /var/backups/repotest/observation-last-export.new
    mv /var/backups/repotest/observation-last-export.new /var/backups/repotest/observation-last-export.txt
    printf 'OBSERVATION_EXPORT_PASS %s %s %s\n' "$since" "$until" "$archive"
else
    printf 'OBSERVATION_EXPORT_FAIL %s %s %s\n' "$since" "$until" "$archive" >&2
    exit 1
fi
```

Record interval/count and command exit status, and check for lost/rotated entries before accepting coverage; exit 0 alone is not complete evidence. The journal's own rate limits and rotation are not an archive guarantee. The timer's `Persistent=true` can catch up one missed activation, but cannot manufacture missing five-minute samples or seven complete days. No raw main-service journal belongs in the shared archive.
- [ ] Record one scheduled health result every five minutes, service `ActiveState/SubState/NRestarts`, and `df` for both DB/root and work filesystems. Record start/end times, expected versus actual sample count, missing samples and all incidents. Keep observation archives private and bounded; never include pairing-code journal lines in shared evidence. A larger interval or missing periods must be recorded rather than called seven complete days.
- [ ] After seven actual consecutive days, rerun cumulative Windows/Linux smoke and review install/drill/observation evidence independently. S5 remains in progress until these checks pass; a timer setup alone does not close ISSUE-010.

## Rollback and failure containment

Installation is staged and records each created resource. A failed install is not a successful deployment: stop/disable only this new `repotest-health.timer`, `repotest-health.service`, `repotest.service` and `var-lib-repotest-work.mount`, verify the mount is detached, retain the image, database, backups, unit files and diagnostics for review. Do not delete the new account or data automatically. For this first installation there is no previous service binary to roll back to; preserving the installation and leaving it stopped is the reversible recovery point. Cleanup of exact new resources requires its own explicit selection; never recurse across an active mount or alter Docker/unrelated services. Restore rollback uses the complete saved DB set described above. A host reboot is outside this plan's authorization.
