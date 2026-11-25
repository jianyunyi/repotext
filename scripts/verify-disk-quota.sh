#!/bin/sh
set -eu

image=/data/home/repotest-s4-quota.img
mountpoint=/data/home/repotest-s4-quota
if test -e "$image" || mountpoint -q "$mountpoint" || test -e "$mountpoint"; then
	echo 'quota probe path already exists' >&2
	exit 2
fi
cleanup() {
	sudo -n umount "$mountpoint" >/dev/null 2>&1 || true
	rmdir "$mountpoint" >/dev/null 2>&1 || true
	rm -f "$image"
}
trap cleanup EXIT HUP INT TERM

truncate -s 2G "$image"
/sbin/mkfs.ext4 -q "$image"
mkdir "$mountpoint"
sudo -n mount -o loop "$image" "$mountpoint"
sudo -n chown "$(id -u):$(id -g)" "$mountpoint"
chmod 700 "$mountpoint"
printf 'QUOTA_MOUNT\n'
df -B1 "$mountpoint"
printf 'QUOTA_PRESSURE\n'
if dd if=/dev/zero of="$mountpoint/fill" bs=1M count=2304 conv=fsync 2>"$mountpoint/dd.log"; then
	echo 'QUOTA_PRESSURE_NOT_ENFORCED' >&2
	exit 3
else
	rc=$?
fi
printf 'QUOTA_PRESSURE_RC=%s\n' "$rc"
test "$rc" -ne 0
rm -f "$mountpoint/fill" "$mountpoint/dd.log"
printf 'QUOTA_PRESSURE_PASS\n'

REPOTEST_E2E_WORK_DIR="$mountpoint" sh scripts/verify-http-runner.sh
test -z "$(find "$mountpoint" -mindepth 1 -maxdepth 1 ! -name lost+found -print -quit)"
printf 'QUOTA_HTTP_WORKSPACE_CLEAN\n'
