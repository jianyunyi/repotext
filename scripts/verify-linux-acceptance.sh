#!/bin/sh
set -eu
export GOMAXPROCS=2
export REPOTEST_TEST_IMAGE=sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414
export REPOTEST_TEST_GITHUB=1
printf 'TARGET '; uname -srmo
go version
docker image inspect "$REPOTEST_TEST_IMAGE" --format '{{.Id}}'
owner="io.repotest.owner=repotest-s4-v1-uid-$(id -u)"
printf 'PREEXISTING_RUNNER_CONTAINERS\n'
docker ps -aq --filter "label=$owner"
printf 'SANDBOX_PROFILE_BEGIN\n'
sh scripts/verify-sandbox-profile.sh
printf 'SANDBOX_PROFILE_PASS\n'
printf 'GO_TEST_BEGIN\n'
go test -p=1 -count=1 -v -timeout=12m ./...
printf 'GO_TEST_PASS\n'
go vet ./...
printf 'GO_VET_PASS\n'
go build ./...
printf 'GO_BUILD_PASS\n'
printf 'S5_OPERATIONS_BEGIN\n'
sh scripts/verify-s5-operations.sh
printf 'HTTP_RUNNER_E2E_BEGIN\n'
sh scripts/verify-disk-quota.sh
printf 'HTTP_RUNNER_E2E_PASS\n'
printf 'FINAL_RUNNER_CONTAINERS\n'
remaining=$(docker ps -aq --filter "label=$owner")
test -z "$remaining"
printf 'FINAL_RUNNER_NETWORKS\n'
remaining=$(docker network ls -q --filter "label=$owner")
test -z "$remaining"
printf 'RUNNER_RESOURCES_CLEAN\n'
