#!/bin/sh
# Collect target facts and prepare reviewed bytes; no service installation.
set -eu
umask 077

test "$(uname -s)" = Linux || { echo 'S5_PREPARATION_FAIL: Linux is required' >&2; exit 1; }
printf 'TARGET '; uname -srmo
printf 'OPERATOR '; id
printf 'PID1 '; cat /proc/1/comm
printf 'SYSTEMD_RUNTIME '
if test -d /run/systemd/system; then echo present; else echo absent; fi
printf 'SYSTEMCTL_STATE_BEGIN\n'
if command -v systemctl >/dev/null 2>&1; then
	systemctl is-system-running 2>&1 || true
	systemctl show repotest.service -p LoadState -p ActiveState -p SubState -p MainPID -p FragmentPath 2>&1 || true
else
	echo 'SYSTEMCTL_UNAVAILABLE'
fi
printf 'SYSTEMCTL_STATE_END\n'
printf 'SUDO_PROBE='
if test "$(id -u)" = 0; then
	echo root_uid
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
	echo true_noninteractive
else
	echo unavailable
fi
printf 'SUDO_POLICY_BEGIN\n'
if command -v sudo >/dev/null 2>&1; then sudo -n -l 2>&1 || true; else echo 'SUDO_UNAVAILABLE'; fi
printf 'SUDO_POLICY_END\n'
echo 'INSTALL_COMMAND_PERMISSIONS=UNVERIFIED'
printf 'REPOTEST_ACCOUNT '; getent passwd repotest || true
printf 'REPOTEST_GROUP '; getent group repotest || true
printf 'DOCKER_GROUP '; getent group docker || true
printf 'LISTENERS_BEGIN\n'
if command -v ss >/dev/null 2>&1; then ss -ltn '( sport = :8080 )'; else echo 'SS_UNAVAILABLE'; fi
printf 'LISTENERS_END\n'
for path in /etc/systemd/system/repotest.service /etc/repotest /var/lib/repotest /var/backups/repotest /usr/local/libexec/repotest /usr/local/libexec/repotest-backup; do
	if test -e "$path" || test -L "$path"; then
		stat -c 'EXISTING_PATH %F %a %U:%G %n' "$path"
	else
		printf 'MISSING_PATH %s\n' "$path"
	fi
done

build_dir=$(mktemp -d /tmp/repotest-s5-prepared.XXXXXXXX)
printf 'PREPARED_DIR=%s\n' "$build_dir"
chmod 700 "$build_dir"
go build -o "$build_dir/repotest" ./cmd/repotest
go build -o "$build_dir/repotest-backup" ./cmd/repotest-backup
cp deploy/repotest.service deploy/repotest.env.example scripts/check-health.sh scripts/check-deployment.sh "$build_dir/"
cd "$build_dir"
for script in ./*.sh; do sh -n "$script"; done
sha256sum repotest repotest-backup repotest.service repotest.env.example check-health.sh check-deployment.sh > artifacts.sha256
sha256sum -c artifacts.sha256
cat artifacts.sha256
printf 'S5_PREPARATION_PASS\n'
printf 'INSTALLATION=NOT_RUN RESTORE=NOT_RUN OBSERVATION=NOT_STARTED\n'
