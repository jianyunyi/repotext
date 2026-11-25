#!/bin/sh
set -eu

test "$(uname -s)" = Linux || { echo 'S5_OPERATIONS_FAIL: Linux is required' >&2; exit 1; }
fixture=$(mktemp -d /tmp/repotest-s5.XXXXXXXX)
trap 'rm -rf -- "$fixture"' EXIT HUP INT TERM
mkdir "$fixture/db" "$fixture/work"
chmod 700 "$fixture" "$fixture/db" "$fixture/work"

go test -p=1 -count=1 -v ./cmd/repotest-backup
echo 'S5_BACKUP_PASS'
python3 scripts/test-check-health.py
echo 'S5_HEALTH_PASS'

export REPOTEST_DB="$fixture/db/repotest.db"
export REPOTEST_WORK_DIR="$fixture/work"
export REPOTEST_ADDR=127.0.0.1:8080
export REPOTEST_LAN_ADDR=
export REPOTEST_RUNNER_ENABLED=0
export REPOTEST_AC008_ACCEPTED=0
export REPOTEST_GO_IMAGE=
sh scripts/check-deployment.sh >"$fixture/valid.log"
grep -q '^DEPLOYMENT_PREFLIGHT_PASS$' "$fixture/valid.log"
grep -q '^RUNNER_DISABLED=1$' "$fixture/valid.log"
echo 'S5_PREFLIGHT_PRIVATE_PASS'

expect_rejected() {
	label=$1
	shift
	if "$@" >"$fixture/$label.log" 2>&1; then
		echo "S5_OPERATIONS_FAIL: $label was accepted" >&2
		exit 1
	fi
	grep -q '^DEPLOYMENT_PREFLIGHT_FAIL:' "$fixture/$label.log"
	echo "S5_PREFLIGHT_${label}_PASS"
}

expect_rejected RELATIVE env REPOTEST_DB=relative.db sh scripts/check-deployment.sh
ln -s "$fixture/work" "$fixture/work-link"
expect_rejected SYMLINK env REPOTEST_WORK_DIR="$fixture/work-link" sh scripts/check-deployment.sh
expect_rejected SYMLINK_TRAILING env REPOTEST_WORK_DIR="$fixture/work-link/" sh scripts/check-deployment.sh
ln -s "$fixture" "$fixture/ancestor-link"
expect_rejected SYMLINK_ANCESTOR env REPOTEST_WORK_DIR="$fixture/ancestor-link/work" sh scripts/check-deployment.sh
expect_rejected SYMLINK_ANCESTOR_TRAILING env REPOTEST_WORK_DIR="$fixture/ancestor-link/work/" sh scripts/check-deployment.sh
expect_rejected DB_SYMLINK_ANCESTOR env REPOTEST_DB="$fixture/ancestor-link/db/repotest.db" sh scripts/check-deployment.sh
ln -s "$fixture/db/missing.db" "$fixture/db/repotest.db"
expect_rejected DB_DANGLING_SYMLINK sh scripts/check-deployment.sh
rm -- "$fixture/db/repotest.db"
chmod 750 "$fixture/work"
expect_rejected PERMISSIONS sh scripts/check-deployment.sh
chmod 700 "$fixture/work"
expect_rejected RUNNER_GATE env REPOTEST_RUNNER_ENABLED=1 sh scripts/check-deployment.sh
expect_rejected NON_LOOPBACK env REPOTEST_ADDR=0.0.0.0:8080 sh scripts/check-deployment.sh
expect_rejected PORT_EMPTY env REPOTEST_ADDR=127.0.0.1: sh scripts/check-deployment.sh
expect_rejected PORT_NON_NUMERIC env REPOTEST_ADDR=127.0.0.1:invalid sh scripts/check-deployment.sh
expect_rejected PORT_NEGATIVE env REPOTEST_ADDR=127.0.0.1:-1 sh scripts/check-deployment.sh
expect_rejected PORT_OUT_OF_RANGE env REPOTEST_ADDR='[::1]:65536' sh scripts/check-deployment.sh
expect_rejected PORT_USERINFO env REPOTEST_ADDR=127.0.0.1:8080@example.com sh scripts/check-deployment.sh
env REPOTEST_ADDR='[::1]:0' sh scripts/check-deployment.sh >"$fixture/ipv6-zero.log"
grep -q '^DEPLOYMENT_PREFLIGHT_PASS$' "$fixture/ipv6-zero.log"
echo 'S5_PREFLIGHT_IPV6_ZERO_PORT_PASS'
expect_rejected LAN env REPOTEST_LAN_ADDR=0.0.0.0:8081 sh scripts/check-deployment.sh

# These checks inspect the supplied contract; they do not install or start it.
grep -qx 'User=repotest' deploy/repotest.service
grep -qx 'Group=repotest' deploy/repotest.service
grep -qx 'ReadWritePaths=/var/lib/repotest' deploy/repotest.service
grep -qx 'REPOTEST_RUNNER_ENABLED=0' deploy/repotest.env.example
grep -qx 'REPOTEST_AC008_ACCEPTED=0' deploy/repotest.env.example
grep -qx 'REPOTEST_LAN_ADDR=' deploy/repotest.env.example
echo 'S5_SYSTEMD_CONTRACT_STATIC_PASS'
echo 'S5_OPERATIONS_PASS'
