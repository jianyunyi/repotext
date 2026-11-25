#!/bin/sh
set -eu

fail() {
	echo "DEPLOYMENT_PREFLIGHT_FAIL: $*" >&2
	exit 1
}

is_absolute() {
	case "$1" in
		/*) return 0 ;;
		*) return 1 ;;
	esac
}

reject_symlinks() {
	path=$1
	while :; do
		while test "$path" != / && test "${path%/}" != "$path"; do
			path=${path%/}
		done
		test ! -L "$path" || fail "path contains a symlink: $path"
		test "$path" != / || return 0
		path=$(dirname "$path") || fail "cannot inspect path ancestors: $path"
	done
}

private_dir() {
	path=$1
	test -d "$path" || fail "directory does not exist: $path"
	test ! -L "$path" || fail "directory is a symlink: $path"
	mode=$(stat -c '%a' "$path") || fail "cannot stat directory: $path"
	if awk -v m="$mode" 'BEGIN { g=int(m / 10) % 10; o=m % 10; exit !(g != 0 || o != 0) }'; then
		fail "directory must be private: $path"
	fi
}

db=${REPOTEST_DB:-}
work=${REPOTEST_WORK_DIR:-}
test -n "$db" || fail 'REPOTEST_DB is required'
test -n "$work" || fail 'REPOTEST_WORK_DIR is required'
is_absolute "$db" || fail 'REPOTEST_DB must be absolute'
is_absolute "$work" || fail 'REPOTEST_WORK_DIR must be absolute'
reject_symlinks "$db"
reject_symlinks "$work"
private_dir "$(dirname "$db")"
private_dir "$work"
if test -e "$db"; then
	test -f "$db" || fail "database is not a regular file: $db"
	test ! -L "$db" || fail "database is a symlink: $db"
	mode=$(stat -c '%a' "$db") || fail "cannot stat database: $db"
	if awk -v m="$mode" 'BEGIN { g=int(m / 10) % 10; o=m % 10; exit !(g != 0 || o != 0) }'; then
		fail "database must be private: $db"
	fi
fi

addr=${REPOTEST_ADDR:-127.0.0.1:8080}
case "$addr" in
	127.0.0.1:*) port=${addr#127.0.0.1:} ;;
	\[::1\]:*) port=${addr#\[::1\]:} ;;
	*) fail "REPOTEST_ADDR must be an explicit loopback address" ;;
esac
case "$port" in
	''|*[!0-9]*) fail 'REPOTEST_ADDR must have a numeric port' ;;
esac
awk -v p="$port" 'BEGIN { exit !(p >= 0 && p <= 65535) }' || fail 'REPOTEST_ADDR port must be between 0 and 65535'
test -z "${REPOTEST_LAN_ADDR:-}" || fail 'REPOTEST_LAN_ADDR must be empty for the default deployment'

enabled=${REPOTEST_RUNNER_ENABLED:-0}
accepted=${REPOTEST_AC008_ACCEPTED:-0}
if test "$enabled" = 1; then
	test "$accepted" = 1 || fail 'runner requires REPOTEST_AC008_ACCEPTED=1'
	image=${REPOTEST_GO_IMAGE:-}
	printf '%s' "$image" | grep -Eq '^sha256:[0-9a-f]{64}$' || fail 'runner image must be a lowercase immutable digest'
else
	echo 'RUNNER_DISABLED=1'
fi

echo 'DEPLOYMENT_PREFLIGHT_PASS'
