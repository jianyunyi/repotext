"""Exercise the PowerShell driver with local child processes; no SSH or Linux run."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

source = Path(__file__).with_name('invoke-linux-acceptance.ps1').read_text(encoding='utf-8-sig')
failures = []
for label, remote_exit, consume, expected_exit in (
    ('early failure', 255, False, 255),
    ('successful transfer', 0, True, 0),
    ('early zero exit', 0, False, 1),
):
    with tempfile.TemporaryDirectory(prefix='repotest-driver-') as directory:
        root = Path(directory)
        for part in ('cmd', 'internal', 'scripts', 'deploy'):
            (root / part).mkdir()
        (root / 'go.mod').write_text('module fixture\n')
        (root / 'go.sum').write_text('')
        # Larger than the pipe buffer, so an early child exit interrupts CopyTo.
        (root / 'internal/payload.bin').write_bytes(os.urandom(2 * 1024 * 1024))
        fixture = root / 'child.py'
        fixture.write_text(
            'import hashlib, sys\n'
            + ('data = sys.stdin.buffer.read()\nprint("TRANSFER_SHA256=" + hashlib.sha256(data).hexdigest())\n' if consume else '')
            + 'print("fixture stdout", flush=True)\n'
            + 'print("fixture stderr", file=sys.stderr, flush=True)\n'
            + f'sys.exit({remote_exit})\n'
        )
        lines = source.splitlines()
        for index, line in enumerate(lines):
            if line.startswith('$startInfo.FileName = '):
                lines[index] = "$startInfo.FileName = '" + sys.executable.replace("'", "''") + "'"
            if line.startswith('$startInfo.Arguments = '):
                lines[index] = "$startInfo.Arguments = '\"" + str(fixture).replace("'", "''") + "\"'"
        driver = root / 'scripts/invoke-linux-acceptance.ps1'
        driver.write_text('\n'.join(lines) + '\n', encoding='utf-8')
        completed = subprocess.run(
            ['powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(driver), '-EvidenceName', 'fixture'],
            capture_output=True, timeout=30,
        )
        evidence = root / 'work/fixture'
        try:
            result = json.loads((evidence / 'result.json').read_text(encoding='utf-8-sig'))
            assert completed.returncode == result['exit_code'] == expected_exit
            assert result['remote_exit_code'] == remote_exit
            assert bool(result['transfer_error']) == (not consume)
            assert 'fixture stdout' in (evidence / 'stdout.log').read_text()
            assert 'fixture stderr' in (evidence / 'stderr.log').read_text()
            if consume:
                import hashlib
                digest = hashlib.sha256((evidence / 'source.tar.gz').read_bytes()).hexdigest()
                assert 'TRANSFER_SHA256=' + digest in (evidence / 'stdout.log').read_text()
            print('PASS ' + label)
        except (AssertionError, FileNotFoundError, KeyError) as error:
            print('FAIL ' + label + ': ' + str(error))
            failures.append(label)
if failures:
    raise SystemExit('driver regression failures: ' + ', '.join(failures))
