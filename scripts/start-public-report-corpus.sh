#!/bin/sh
# Retain a private validation workspace across SSH disconnects.
set -eu
test "$(uname -s)" = Linux
run=$(mktemp -d /data/home/repotest-public-corpus.XXXXXXXX)
cp -R cmd internal scripts deploy go.mod go.sum source.sha256 "$run/"
cd "$run"
nohup sh -c 'sh scripts/verify-public-report-corpus.sh >run.stdout.log 2>run.stderr.log; result=$?; printf "%s\n" "$result" >run.exit' </dev/null >/dev/null 2>&1 &
printf 'PUBLIC_CORPUS_REMOTE=%s\n' "$run"
