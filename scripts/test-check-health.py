"""Run with python3; --shell selects a POSIX shell on Windows."""

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading


parser = argparse.ArgumentParser()
parser.add_argument("--shell", default="sh")
args = parser.parse_args()
requests = []


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        requests.append(self.path)
        self.send_response(201 if self.path == "/created" else 200)
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')

    def log_message(self, *args):
        pass


server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
thread = threading.Thread(target=server.serve_forever, daemon=True)
thread.start()
script = Path(__file__).with_name("check-health.sh").as_posix()
base = f"http://127.0.0.1:{server.server_port}"
failures = []
try:
    for label, url, success, must_not_request in [
        ("HTTP 200", base + "/healthz", True, False),
        ("HTTP 201", base + "/created", False, False),
        ("userinfo", "http://127.0.0.1:8080@" + base[7:] + "/healthz", False, True),
        ("hostname", "http://localhost:8080/healthz", False, True),
    ]:
        before = len(requests)
        result = subprocess.run([args.shell, script, url], capture_output=True, text=True, timeout=15)
        marker = "HEALTH_PASS" if success else "HEALTH_FAIL"
        passed = (result.returncode == 0) == success and marker in result.stdout + result.stderr
        passed = passed and (not must_not_request or len(requests) == before)
        print(f"{'PASS' if passed else 'FAIL'} {label}: exit={result.returncode}")
        if not passed:
            failures.append(label)
finally:
    server.shutdown()
    server.server_close()
    thread.join()
if failures:
    raise SystemExit("health regression failures: " + ", ".join(failures))
