#!/bin/sh
set -eu

image='busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662'
if ! docker image inspect "$image" >/dev/null 2>&1; then
	printf 'required local probe image is missing: %s (pulling is disabled)\n' "$image" >&2
	exit 2
fi

docker run --rm --pull=never \
	--network none \
	--memory=128m \
	--cpus=0.5 \
	--pids-limit=32 \
	--read-only \
	--cap-drop=ALL \
	--security-opt=no-new-privileges \
	--user=65534:65534 \
	--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=16m \
	"$image" sh -c '
set -eu
cpu=$(cat /sys/fs/cgroup/cpu.max)
memory=$(cat /sys/fs/cgroup/memory.max)
pids=$(cat /sys/fs/cgroup/pids.max)
printf "CPU_MAX=%s\nMEMORY_MAX=%s\nPIDS_MAX=%s\n" "$cpu" "$memory" "$pids"
test "$cpu" = "50000 100000"
test "$memory" = 134217728
test "$pids" = 32
grep -q "^CapEff:[[:space:]]*0000000000000000$" /proc/self/status
echo CAPABILITIES_DROPPED
if touch /etc/repotest-write-probe 2>/dev/null; then
	echo ROOTFS_WRITABLE >&2
	exit 4
fi
echo ROOTFS_READ_ONLY
test "$(grep -c "^[[:space:]]*lo:" /proc/net/dev)" -eq 1
echo NETWORK_LOOPBACK_ONLY
if dd if=/dev/zero of=/tmp/fill bs=1M count=20 2>/dev/null; then
	echo TMPFS_LIMIT_NOT_ENFORCED >&2
	exit 5
fi
echo TMPFS_16M_LIMIT_ENFORCED

before_cpu=$(awk "/^nr_throttled / {print \$2}" /sys/fs/cgroup/cpu.stat)
(while :; do :; done) &
spinner=$!
sleep 1
kill "$spinner"
wait "$spinner" 2>/dev/null || true
after_cpu=$(awk "/^nr_throttled / {print \$2}" /sys/fs/cgroup/cpu.stat)
test "$after_cpu" -gt "$before_cpu"
printf "CPU_THROTTLED_DELTA=%s\n" "$((after_cpu - before_cpu))"

read -r label before_pids < /sys/fs/cgroup/pids.events
test "$label" = max
sh -c "i=0; while [ \$i -lt 40 ]; do sleep 2 & i=\$((i + 1)); done; wait" 2>/dev/null || true
read -r label after_pids < /sys/fs/cgroup/pids.events
test "$label" = max
test "$after_pids" -gt "$before_pids"
printf "PID_LIMIT_REJECTION_DELTA=%s\n" "$((after_pids - before_pids))"
'

memory_probe="repotest-memory-probe-$$"
if docker container inspect "$memory_probe" >/dev/null 2>&1; then
	echo "probe container name already exists: $memory_probe" >&2
	exit 7
fi
cleanup_memory_probe() {
	docker rm -f "$memory_probe" >/dev/null 2>&1 || true
}
trap cleanup_memory_probe EXIT HUP INT TERM

if docker run --pull=never --name "$memory_probe" \
	--network none \
	--memory=64m \
	--memory-swap=64m \
	--pids-limit=8 \
	--read-only \
	--cap-drop=ALL \
	--security-opt=no-new-privileges \
	--user=65534:65534 \
	--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=128m \
	"$image" sh -c 'dd if=/dev/zero of=/tmp/fill bs=1M count=96 2>/dev/null'; then
	echo MEMORY_LIMIT_NOT_ENFORCED >&2
	exit 6
else
	status=$?
fi
oom_killed=$(docker container inspect --format '{{.State.OOMKilled}}' "$memory_probe")
exit_code=$(docker container inspect --format '{{.State.ExitCode}}' "$memory_probe")
printf 'MEMORY_PRESSURE_EXIT=%s OOM_KILLED=%s\n' "$exit_code" "$oom_killed"
test "$status" -eq 137
test "$exit_code" -eq 137
test "$oom_killed" = true
echo MEMORY_64M_LIMIT_ENFORCED
docker rm "$memory_probe" >/dev/null
trap - EXIT HUP INT TERM
