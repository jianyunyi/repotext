#!/bin/sh
set -eu
D=$(mktemp -d /tmp/repotest-http-e2e.XXXXXXXX)
cleanup() { if test -n "${PID:-}"; then kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; fi; rm -rf "$D"; }
trap cleanup EXIT HUP INT TERM
work_dir=${REPOTEST_E2E_WORK_DIR:-$D/work}
if test -z "${REPOTEST_E2E_WORK_DIR:-}"; then
	mkdir -p "$work_dir"
fi
chmod 700 "$work_dir"
go build -o "$D/repotest" ./cmd/repotest
image='sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414'
port=29201
REPOTEST_DB="$D/data.db" REPOTEST_ADDR="127.0.0.1:$port" REPOTEST_RUNNER_ENABLED=1 REPOTEST_AC008_ACCEPTED=1 REPOTEST_GO_IMAGE="$image" REPOTEST_WORK_DIR="$work_dir" "$D/repotest" >"$D/server.log" 2>&1 &
PID=$!
for i in $(seq 1 60); do
	if curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; then break; fi
	sleep 1
done
curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null
page=$(curl -fsS "http://127.0.0.1:$port/")
token=$(printf '%s' "$page" | sed -n 's/.*name="repotest-csrf" content="\([^"]*\)".*/\1/p')
test -n "$token"
origin="http://127.0.0.1:$port"
repo='https://github.com/gorilla/mux'
sha='b4617d0b9670ad14039b2739167fd35a60f557c5'
preview=$(curl -fsS -H "Origin: $origin" -H "X-CSRF-Token: $token" -H 'Content-Type: application/json' --data "{\"repo_url\":\"$repo\",\"ref\":\"$sha\"}" "http://127.0.0.1:$port/api/v1/previews")
preview_id=$(printf '%s' "$preview" | python3 -c 'import json,sys; print(json.load(sys.stdin)["preview_id"])')
confirmed=$(curl -fsS -H "Origin: $origin" -H "X-CSRF-Token: $token" -H 'Content-Type: application/json' --data "{\"preview_id\":\"$preview_id\"}" "http://127.0.0.1:$port/api/v1/tasks")
task_id=$(printf '%s' "$confirmed" | python3 -c 'import json,sys; print(json.load(sys.stdin)["task"]["id"])')
status=queued
for i in $(seq 1 180); do
	task=$(curl -fsS "http://127.0.0.1:$port/api/v1/tasks/$task_id")
	status=$(printf '%s' "$task" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
	case "$status" in succeeded|failed|timeout|cancelled|error) break;; esac
	sleep 1
done
test "$status" = succeeded
report=$(curl -fsS "http://127.0.0.1:$port/api/v1/tasks/$task_id/report")
printf '%s' "$report" | python3 -c 'import json,sys; r=json.load(sys.stdin); assert r["task_id"] and r["packages"]'
printf 'HTTP_TASK_SUCCESS status=%s task=%s\n' "$status" "$task_id"
preview=$(curl -fsS -H "Origin: $origin" -H "X-CSRF-Token: $token" -H 'Content-Type: application/json' --data "{\"repo_url\":\"$repo\",\"ref\":\"db9d1d0073d27a0a2d9a8c1bc52aa0af4374d265\"}" "http://127.0.0.1:$port/api/v1/previews")
preview_id=$(printf '%s' "$preview" | python3 -c 'import json,sys; print(json.load(sys.stdin)["preview_id"])')
confirmed=$(curl -fsS -H "Origin: $origin" -H "X-CSRF-Token: $token" -H 'Content-Type: application/json' --data "{\"preview_id\":\"$preview_id\"}" "http://127.0.0.1:$port/api/v1/tasks")
cancel_id=$(printf '%s' "$confirmed" | python3 -c 'import json,sys; print(json.load(sys.stdin)["task"]["id"])')
cancel_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H "Origin: $origin" -H "X-CSRF-Token: $token" "http://127.0.0.1:$port/api/v1/tasks/$cancel_id/cancel")
test "$cancel_status" = 204
for i in $(seq 1 30); do
	task=$(curl -fsS "http://127.0.0.1:$port/api/v1/tasks/$cancel_id")
	status=$(printf '%s' "$task" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
	if test "$status" = cancelled; then break; fi
	sleep 1
done
test "$status" = cancelled
printf 'HTTP_TASK_CANCEL status=%s task=%s\n' "$status" "$cancel_id"
