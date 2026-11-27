#!/bin/sh
# Smoke checks for report changes in the existing controlled test boundary.
set -eu
export GOMAXPROCS=2 GOTOOLCHAIN=local
export REPOTEST_TEST_IMAGE=sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414
owner="io.repotest.owner=repotest-s4-v1-uid-$(id -u)"
test -z "$(docker ps -aq --filter "label=$owner")"
test -z "$(docker network ls -q --filter "label=$owner")"
uname -srmo
go version
go test -p=1 -count=1 -v -timeout=5m -run 'Test(.*Report.*|OfflineGoInDocker|.*Dependency.*)' ./internal/runner
go test -p=1 -count=1 -timeout=5m ./internal/control ./internal/domain ./cmd/...
go vet ./...
go build ./...
test -z "$(docker ps -aq --filter "label=$owner")"
test -z "$(docker network ls -q --filter "label=$owner")"
printf 'PUBLIC_REPORT_LINUX_SMOKE_PASS\n'
