"""Local transport checks for the corpus SSH driver; no SSH/network calls."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

source = Path(__file__).with_name('invoke-public-corpus-target.ps1').read_text(encoding='utf-8-sig')
for label, child_exit, consume, expected_exit in [('bytes', 0, True, 0), ('remote-failure', 255, False, 255), ('early-zero', 0, False, 1)]:
    with tempfile.TemporaryDirectory(prefix='repotest-corpus-driver-') as directory:
        root = Path(directory)
        (root / 'scripts').mkdir()
        (root / 'work').mkdir()
        # Force a pipe write larger than the pipe buffer for early-exit checks.
        payload = 'set -eu\n# 中文\n' + os.urandom(2 << 20).hex() + '\n'
        script = root / 'work/probe.sh'
        script.write_text(payload, encoding='utf-8-sig')
        child = root / 'child.py'
        child.write_text('import sys, hashlib\n' + ('data=sys.stdin.buffer.read()\nprint("INPUT_SHA="+hashlib.sha256(data).hexdigest())\n' if consume else '') + 'print("child stdout")\nprint("child stderr",file=sys.stderr)\n' + f'sys.exit({child_exit})\n')
        lines = source.splitlines()
        for i, line in enumerate(lines):
            if line.startswith('$info.FileName='):
                lines[i] = "$info.FileName='" + sys.executable.replace("'", "''") + "'"
            if line.startswith('$info.Arguments='):
                lines[i] = "$info.Arguments='\"" + str(child).replace("'", "''") + "\"'"
        driver = root / 'scripts/invoke-public-corpus-target.ps1'
        driver.write_text('\n'.join(lines), encoding='utf-8-sig')
        completed = subprocess.run(['powershell','-NoProfile','-ExecutionPolicy','Bypass','-File',str(driver),'-ScriptPath',str(script),'-EvidenceName','fixture'],capture_output=True,timeout=30)
        evidence = root / 'work/fixture'
        result = json.loads((evidence / 'result.json').read_text(encoding='utf-8-sig'))
        assert completed.returncode == result['exit_code'] == expected_exit, (label, completed.stderr, result)
        assert bool(result['transfer_error']) == (not consume)
        stdout = (evidence / 'stdout.log').read_text(encoding='utf-8-sig')
        assert 'child stdout' in stdout
        assert 'child stderr' in (evidence / 'stderr.log').read_text(encoding='utf-8-sig')
        if consume:
            assert 'INPUT_SHA='+hashlib.sha256(payload.encode()).hexdigest() in stdout
        print('PASS '+label)
