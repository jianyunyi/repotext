#!/bin/sh
set -eu

fail() {
	echo "HEALTH_FAIL: $*" >&2
	exit 1
}

url=${1:-}
test -n "$url" || fail 'usage: check-health.sh http://loopback:port/healthz'
authority=${url#http://}
test "$authority" != "$url" || fail 'health URL must use HTTP'
authority=${authority%%/*}
case "$authority" in
	127.0.0.1:*) port=${authority#127.0.0.1:} ;;
	\[::1\]:*) port=${authority#\[::1\]:} ;;
	*) fail 'health URL must use an explicit loopback address' ;;
esac
case "$port" in
	''|*[!0-9]*) fail 'health URL must use a numeric loopback port without userinfo' ;;
esac

response=$(curl --silent --show-error --fail --max-time 5 --connect-timeout 2 --proto '=http' --write-out '\n%{http_code}' -- "$url") || fail 'health request failed'
status=$(printf '%s\n' "$response" | tail -n 1)
test "$status" = 200 || fail "health request returned HTTP $status"
body=$(printf '%s\n' "$response" | sed '$d')
case "$body" in
	*'"status":"ok"'*) echo 'HEALTH_PASS' ;;
	*) fail 'health response did not report status ok' ;;
esac
