#!/bin/sh
# Temporary, sequential pinned-repository validation. No service is started.
set -eu
export GOMAXPROCS=2
export GOTOOLCHAIN=local
export REPOTEST_TEST_IMAGE=sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414
export REPOTEST_PUBLIC_CORPUS=1
export REPOTEST_CORPUS_OUT="$PWD/corpus-evidence"
owner="io.repotest.owner=repotest-s4-v1-uid-$(id -u)"
# Abort if another managed run is active; never clean another run's resources.
test -z "$(docker ps -aq --filter "label=$owner")"
test -z "$(docker network ls -q --filter "label=$owner")"
uname -srmo
go version
set +e
go test -p=1 -count=1 -v -timeout=60m -run '^TestPublicRepositoryReportCorpus$' ./internal/runner
result=$?
set -e
test -z "$(docker ps -aq --filter "label=$owner")" || result=1
test -z "$(docker network ls -q --filter "label=$owner")" || result=1
if test -d corpus-evidence; then
  tar -czf corpus-evidence.tar.gz corpus-evidence
fi
printf 'PUBLIC_CORPUS_EXIT=%s\n' "$result"
exit "$result"
