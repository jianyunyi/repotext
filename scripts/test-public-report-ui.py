"""Repeatable real Edge checks. All reports and HTTP responses below are labelled fixtures."""
import argparse
import json
from pathlib import Path
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "internal/control/web"
MALICIOUS = '<img src=x onerror="window.fixtureXSS=1">'
SHA = "a" * 40
SOURCE = {"path": "internal/check_test.go", "line": 12, "github_url": f"https://github.com/acme/demo/blob/{SHA}/internal/check_test.go#L12"}
REPORT = {
    "repo_url": "https://github.com/acme/demo", "commit": SHA, "go_version": MALICIOUS,
    "goos": "linux", "goarch": "amd64", "incomplete": True, "truncated": True,
    "output_truncated": True, "output": MALICIOUS + "\n" + "long-runtime-output" * 300,
    "summary": {"passed": 1, "failed": 1, "skipped": 1, "incomplete": 1, "package_failures": 1, "dropped_cases": 3},
    "packages": [
        {"import_path": "z/passed", "status": "passed", "elapsed_ms": 300, "coverage_pct": 0, "tests": [{"name": "TestPass", "status": "passed", "elapsed_ms": 2}]},
        {"import_path": "b/failure" + MALICIOUS, "status": "failed", "elapsed_ms": 120, "coverage_pct": None,
         "failure_location": SOURCE, "output": MALICIOUS, "output_truncated": True,
         "tests": [{"name": "TestParent", "status": "failed"}, {"name": "TestParent/child" + MALICIOUS, "parent": "TestParent", "status": "failed", "output": MALICIOUS, "failure_location": SOURCE}, {"name": "TestSkip", "status": "skipped"}, {"name": "TestPending", "status": "incomplete"}]},
        {"import_path": "a/no-tests", "status": "no_tests", "elapsed_ms": 20},
    ],
}
LEGACY = {"goos": "linux", "goarch": "amd64", "packages": [{"import_path": "legacy/pkg", "status": "failed", "output": "legacy output"}]}
REQUESTS = []


class FixtureHandler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.respond()

    def do_POST(self):
        self.respond()

    def do_DELETE(self):
        self.respond()

    def respond(self):
        REQUESTS.append((self.command, self.path, dict(self.headers)))
        if self.path == "/":
            body = (WEB / "index.html").read_text(encoding="utf-8").replace("__CSRF_TOKEN__", "fixture-csrf").replace("__EXECUTION_ENABLED__", "true")
            body = body.replace('<main class="shell">', '<main class="shell"><p id="fixture-label">UI QA · synthetic fixture data</p>')
            mime = "text/html; charset=utf-8"
        elif self.path.startswith("/assets/") and self.path.split("/")[-1] in ("report.js", "report.css"):
            asset = WEB / self.path.split("/")[-1]
            body = asset.read_text(encoding="utf-8") if asset.exists() else ""
            mime = "text/javascript" if asset.suffix == ".js" else "text/css"
        elif self.path == "/api/v1/previews":
            body = json.dumps({"full_name": "acme/demo", "default_branch": "main", "ref": "main", "commit_sha": SHA, "repo_url": "https://github.com/acme/demo", "goos": "linux", "goarch": "amd64", "size_kb": 42, "preview_id": "fixture-preview"})
            mime = "application/json"
        elif self.path == "/api/v1/tasks" and self.command == "POST":
            body, mime = json.dumps({"task": {"id": "fixture-task"}}), "application/json"
        elif self.path.endswith("/report"):
            body, mime = json.dumps(REPORT), "application/json"
        elif self.path.endswith("/log"):
            body, mime = MALICIOUS + "\nprivate fixture log", "text/plain"
        elif self.path == "/api/v1/sessions":
            body, mime = "[]", "application/json"
        elif self.command in ("POST", "DELETE"):
            body, mime = "", "text/plain"
        else:
            body, mime = json.dumps({"id": "fixture-task", "status": "failed", "phase": "test"}), "application/json"
        self.send_response(200)
        self.send_header("Content-Type", mime)
        self.end_headers()
        self.wfile.write(body.encode())


def render(page, report):
    page.evaluate("data => { document.querySelector('#qa-report')?.remove(); const target=document.createElement('section'); target.id='qa-report'; document.querySelector('main').append(target); RepoTestReport.render(target,data); }", report)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default="work/public-report-s1/ui")
    parser.add_argument("--screenshots", action="store_true")
    args = parser.parse_args()
    out = ROOT / args.out
    out.mkdir(parents=True, exist_ok=True)
    server = ThreadingHTTPServer(("127.0.0.1", 0), FixtureHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    checks = []
    deep_checks = []
    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(executable_path=r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe", headless=True)
            for viewport in [{"width": 1440, "height": 1000}, {"width": 390, "height": 844}]:
                page = browser.new_page(viewport=viewport)
                errors = []
                page.on("pageerror", lambda error: errors.append(str(error)))
                page.goto(f"http://127.0.0.1:{server.server_port}/")
                assert page.evaluate("typeof window.RepoTestReport?.render === 'function'"), "reusable report renderer is missing"
                deep_cases = [{"name": "TestDeep", "status": "failed"}]
                for depth in range(1, 33):
                    parent = deep_cases[-1]["name"]
                    deep_cases.append({"name": parent + f"/child{depth:02}", "parent": parent, "status": "failed", "output": "synthetic depth regression diagnostic"})
                render(page, {"packages": [{"import_path": "fixture/deep", "status": "failed", "tests": deep_cases}]})
                deep = page.locator("#qa-report")
                deep.evaluate("el => { const result=document.querySelector('#result'); result.className='result show'; result.append(el); }")
                assert deep.locator(".report-test").count() == len(deep_cases), "deep parent fallback dropped cases"
                names = deep.locator(".report-test > summary > .report-name").all_text_contents()
                assert sorted(names) == sorted(test["name"] for test in deep_cases), "deep fallback lost full names"
                assert deep.locator(".report-test[open]").count() == len(deep_cases), "deep failed cases collapsed"
                assert all(summary.inner_text().count("失败") == 1 for summary in deep.locator(".report-test > summary").all()), "deep failed status lost"
                layout = page.evaluate("() => ({viewport: innerWidth, scrollWidth: document.documentElement.scrollWidth, cases: document.querySelectorAll('#qa-report .report-test').length, minWidth: Math.min(...[...document.querySelectorAll('#qa-report .report-test')].map(el => el.getBoundingClientRect().width))})")
                assert layout["scrollWidth"] <= viewport["width"], f"deep subtests overflow: {layout}"
                if viewport["width"] == 390:
                    assert layout["minWidth"] >= 180, f"deep subtests unreadable: {layout}"
                deep_checks.append(layout)
                if args.screenshots:
                    deep.locator(".report-test").last.scroll_into_view_if_needed()
                    page.screenshot(path=str(out / ("deep-subtests-desktop.png" if viewport["width"] == 1440 else "deep-subtests-mobile.png")))
                    page.evaluate("scrollTo(0,0)")
                for capture in ["repo-06", "repo-05"]:
                    actual = json.loads((ROOT / "internal/runner/testdata/public-corpus" / capture / "report.json").read_text(encoding="utf-8"))
                    render(page, actual)
                    captured = page.locator("#qa-report")
                    first = captured.locator(".report-package").first
                    assert first.get_attribute("data-status") in ("failed", "build_failed"), f"{capture}: actual failure was not first"
                    assert first.get_attribute("open") is not None, f"{capture}: actual failure collapsed"
                    assert "失败" in first.locator("summary").first.inner_text(), f"{capture}: persisted failed status has no Chinese label"
                    failed_cases = [test for package in actual["packages"] for test in package.get("tests", []) if test["status"] == "failed"]
                    assert failed_cases and all(captured.locator(".report-test[open]").filter(has_text=test["name"]).count() for test in failed_cases), f"{capture}: failed cases collapsed"
                    if capture == "repo-06":
                        assert "TestJSONString" in captured.inner_text()
                        assert captured.locator('a[href$="/gjson_test.go#L2560"]').count() == 1, "genuine gjson failure source link missing"
                    captured.get_by_label("包状态").select_option("failed")
                    expected = sum(package["status"] in ("failed", "build_failed") for package in actual["packages"])
                    assert captured.locator(".report-package").count() == expected, f"{capture}: failure filter misses actual DTO statuses"
                render(page, REPORT)
                scope = page.locator("#qa-report")
                assert scope.locator("img,script").count() == 0 and page.evaluate("window.fixtureXSS") is None, "untrusted HTML executed"
                assert MALICIOUS in scope.inner_text(), "malicious text was lost"
                assert scope.locator(".report-package").first.get_attribute("data-status") == "failed", "failure-first default missing"
                assert scope.locator(".report-package").first.get_attribute("open") is not None, "failure package collapsed"
                assert scope.locator(".report-test details").count() >= 1, "subtests not nested"
                assert "叶子测试：通过 1" in scope.inner_text() and "包级失败 1" in scope.inner_text(), "summary conflates leaf counts with package failures"
                scope.locator('.report-package[data-status="passed"] > summary').click()
                scope.locator('.report-package[data-status="no_tests"] > summary').click()
                assert "覆盖率 0%" in scope.inner_text() and "覆盖率未提供" in scope.inner_text(), "null coverage conflated with zero"
                assert "未发现测试" in scope.inner_text() and "报告不完整" in scope.inner_text() and "已截断" in scope.inner_text() and "不构成真实性证明" in scope.inner_text(), "limits missing"
                assert scope.locator("a[href]").count() == 2, "valid pinned sources missing"
                scope.get_by_label("包名筛选").fill("no-tests")
                assert scope.locator(".report-package").count() == 1
                scope.get_by_label("包名筛选").fill("")
                scope.get_by_label("包状态").select_option("failed")
                assert scope.locator(".report-package").count() == 1
                scope.get_by_label("包状态").select_option("")
                scope.get_by_label("包排序").select_option("elapsed")
                assert scope.locator(".report-package").first.inner_text().startswith("z/passed"), "elapsed sort missing"
                scope.get_by_label("包排序").select_option("name")
                assert scope.locator(".report-package").first.inner_text().startswith("a/no-tests"), "name sort missing"
                output = scope.locator("pre").first
                assert output.evaluate("el => getComputedStyle(el).whiteSpace === 'pre-wrap' && parseFloat(getComputedStyle(el).maxHeight) < 500"), "output is unbounded or unwrapped"
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "horizontal overflow"
                for bad in [
                    {**SOURCE, "github_url": "javascript:alert(1)"},
                    {**SOURCE, "github_url": SOURCE["github_url"].replace("acme/demo", "evil/demo")},
                    {**SOURCE, "github_url": SOURCE["github_url"].replace(SHA, "main")},
                    {**SOURCE, "path": "../secret.go"},
                    {**SOURCE, "path": "a\\secret.go"},
                    {**SOURCE, "path": "a/%2e%2e/secret.go"},
                    {**SOURCE, "line": 0},
                    {**SOURCE, "github_url": SOURCE["github_url"] + "?redirect=evil"},
                ]:
                    render(page, {"repo_url": REPORT["repo_url"], "commit": SHA, "packages": [{"import_path": "bad/link", "status": "failed", "failure_location": bad}]})
                    assert scope.locator("a[href]").count() == 0, f"unsafe source link accepted {bad}"
                for repo, commit in [("https://github.com.evil/acme/demo", SHA), ("https://user@github.com/acme/demo", SHA), (REPORT["repo_url"], "main")]:
                    render(page, {"repo_url": repo, "commit": commit, "packages": [{"import_path": "bad/meta", "status": "failed", "failure_location": SOURCE}]})
                    assert scope.locator("a[href]").count() == 0, "unsafe report identity accepted"
                render(page, {"repo_url": "https://github.com/acme/..", "commit": SHA, "packages": [{"import_path": "bad/repo", "status": "failed", "failure_location": {"path": "x.go", "line": 12, "github_url": f"https://github.com//blob/{SHA}/x.go#L12"}}]})
                assert scope.locator("a[href]").count() == 0, "normalized repository path accepted"
                render(page, {"packages": [{"import_path": "build/pkg", "status": "build_failed", "output": "fixture compiler diagnostic"}]})
                assert scope.locator(".report-package[open]").count() == 1 and "构建失败" in scope.inner_text()
                scope.get_by_label("包状态").select_option("failed")
                assert scope.locator(".report-package").count() == 1, "failure filter excludes build_failed"
                render(page, LEGACY)
                assert "legacy/pkg" in scope.inner_text() and "legacy output" in scope.inner_text() and "叶子测试汇总未提供" in scope.inner_text(), "legacy report unusable"
                assert "覆盖率 0%" not in scope.inner_text() and "叶子测试：通过 0" not in scope.inner_text(), "fabricated legacy metrics"
                render(page, {"no_tests": True, "packages": []})
                assert "未发现测试" in scope.inner_text()
                page.locator("#qa-report").evaluate("el => el.remove()")
                page.locator("#repo").fill("https://github.com/acme/demo")
                page.locator("#submit").click()
                page.locator("#confirm").click()
                page.locator(".report-package").first.wait_for()
                assert page.locator("#delete-task").is_visible(), "management action lost"
                page.get_by_role("button", name="读取原始日志").click()
                page.locator(".task-log pre").wait_for()
                assert "private fixture log" in page.locator(".task-log").inner_text() and page.locator(".task-log img").count() == 0, "log rendered as HTML"
                assert any(path.endswith("/log") and headers.get("X-CSRF-Token") == "fixture-csrf" for _, path, headers in REQUESTS), "log fetch missing CSRF header"
                page.locator("#refresh-sessions").click()
                page.wait_for_function("document.querySelector('#sessions-result').textContent === '没有有效的已配对设备。'")
                assert page.locator("#sessions-result").inner_text() == "没有有效的已配对设备。"
                assert not errors, errors
                if args.screenshots:
                    page.screenshot(path=str(out / ("desktop.png" if viewport["width"] == 1440 else "mobile.png")), full_page=True)
                page.locator("#delete-task").click()
                page.wait_for_function("document.querySelector('#result').textContent.includes('任务与报告已删除')")
                assert "任务与报告已删除" in page.locator("#result").inner_text()
                cancelled = {"status": "running"}
                page.route("**/api/v1/tasks/cancel-fixture", lambda route: route.fulfill(json={"id": "cancel-fixture", "status": cancelled["status"]}))
                def cancel_route(route):
                    cancelled["status"] = "cancelled"
                    route.fulfill(status=204)
                page.route("**/api/v1/tasks/cancel-fixture/cancel", cancel_route)
                page.evaluate("watchTask('cancel-fixture')")
                page.locator("#cancel-task").click()
                page.wait_for_function("document.querySelector('#result').textContent.includes('已取消')")
                assert page.get_by_role("button", name="读取原始日志").count() == 1, "cancelled task lost private log reader"
                checks.append({"viewport": viewport, "result": "PASS", "fixtures": True})
                page.close()
            browser.close()
        (out / "browser-checks.json").write_text(json.dumps({"checks": checks, "fixture_data": True}, indent=2), encoding="utf-8")
        (out / "deep-subtests-layout.json").write_text(json.dumps(deep_checks, indent=2), encoding="utf-8")
        print("PASS: Edge desktop/mobile genuine gjson/testify report playback, synthetic injection/legacy fixtures, source validation, filter/sort, admin report/log/cancel/delete/session flows")
    finally:
        server.shutdown()


if __name__ == "__main__":
    main()
