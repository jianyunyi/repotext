#!/bin/sh
set -eu
export LC_ALL=C TZ=UTC
umask 077
directory=/var/backups/repotest
archive=uncreated since=unread until=unread
on_exit() {
    rc=$?
    if test "$rc" -ne 0; then
        printf 'OBSERVATION_EXPORT_FAIL %s %s %s\n' "$since" "$until" "$archive" >&2
    fi
}
trap on_exit EXIT
test "$(id -u)" = 0
test -d "$directory" && test ! -L "$directory"
test "$(stat -c '%U:%G:%a' "$directory")" = root:root:700
for path in "$directory/observation-export.lock" "$directory/observation-last-export.txt"; do
    test -f "$path" && test ! -L "$path"
done
exec 9> "$directory/observation-export.lock"
if ! flock -n 9; then echo OBSERVATION_EXPORT_SKIPPED_BUSY; exit 0; fi
since=$(cat "$directory/observation-last-export.txt")
journalctl --sync
until=$(date -u +%Y-%m-%dT%H:%M:%SZ)
archive=$(mktemp "$directory/observation.XXXXXXXX.json")
# Inclusive boundaries overlap intentionally; sample counting deduplicates __CURSOR.
journalctl -u repotest-health.service --since "$since" --until "$until" --no-pager -o json > "$archive"
python3 - "$since" "$until" "$archive" <<'EXPORT_META'
import datetime, json, sys
since, until, archive = sys.argv[1:]
assert datetime.datetime.fromisoformat(until.replace('Z', '+00:00')) >= datetime.datetime.fromisoformat(since.replace('Z', '+00:00'))
count = 0
with open(archive) as source:
    for line in source:
        entry = json.loads(line)
        assert isinstance(entry.get('__CURSOR'), str) and entry['__CURSOR']
        int(entry['__REALTIME_TIMESTAMP'])
        count += 1
with open(archive + '.window.json', 'x') as output:
    json.dump({'unit': 'repotest-health.service', 'since': since, 'until': until,
               'journal_command_exit': 0, 'entries': count,
               'inclusive_boundary_dedup_key': '__CURSOR'}, output)
EXPORT_META
checkpoint_tmp=$(mktemp "$directory/observation-checkpoint.XXXXXXXX")
printf '%s\n' "$until" > "$checkpoint_tmp"
mv -- "$checkpoint_tmp" "$directory/observation-last-export.txt"
printf 'OBSERVATION_EXPORT_PASS %s %s %s\n' "$since" "$until" "$archive"
