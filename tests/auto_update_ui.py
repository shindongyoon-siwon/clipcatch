"""Exercise automatic discovery through the real page scripts and mocked local API.

No render() injection, real release lookup, Windows installation, or download runs.
"""
import json
import os
import shutil
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "src" / "web"
VERSION = (ROOT / "VERSION").read_text().strip()
SCRIPTS = ["premiere.js", "quality.js", "workspace-tabs.js", "startup-update.js"]
html = (WEB / "index.html").read_text().replace("__APP_VERSION__", VERSION)
html = html.replace("</body>", "".join(f'<script src="/{name}"></script>' for name in SCRIPTS) + "</body>", 1)


class LocalAPI:
    def __init__(self):
        self.state = {
            "version": VERSION, "channel": "stable", "phase": "ready", "jobs": [],
            "save_dir": "C:\\Users\\Designer\\Downloads\\ClipCatch", "output_mode": "premiere",
            "app_update": {"status": "current"}, "update_required": False, "update_checking": False,
            "active_tasks": 0, "engine": {"channel": "stable", "message": "사용 가능"},
            "diagnostics": {"checks": []},
        }
        self.release_available = False
        self.offline = False
        self.requests = []

    @property
    def checks(self):
        return [payload for path, payload in self.requests if path == "/api/check-app-update"]

    def current(self):
        self.release_available = False
        self.state.update({"app_update": {"status": "current"}, "update_required": False,
                           "update_checking": False, "jobs": [], "active_tasks": 0})

    def serve(self, route):
        path = urlparse(route.request.url).path
        headers = {"Cache-Control": "no-store"}
        if path == "/":
            route.fulfill(headers=headers, content_type="text/html; charset=utf-8", body=html)
        elif path.endswith(".js") and (WEB / path[1:]).is_file():
            route.fulfill(headers=headers, content_type="application/javascript; charset=utf-8", body=(WEB / path[1:]).read_text())
        elif path == "/api/state":
            route.fulfill(headers=headers, json=self.state)
        elif path == "/api/browser-link/status":
            route.fulfill(json={"paired": True})
        elif path == "/api/quality/status":
            route.fulfill(json={"engine_ready": False, "engine_bundled": False})
        elif route.request.method == "POST":
            self.requests.append((path, route.request.post_data_json))
            if path == "/api/check-app-update":
                if self.offline:
                    route.abort("internetdisconnected")
                    return
                # A fresh release is discoverable only through this endpoint, not /api/state.
                if self.release_available:
                    self.state["update_required"] = True
                    self.state["app_update"] = {
                        "status": "available", "latest": "99.0.0", "notes": "테스트용 새 버전 안내",
                        "release_url": "https://github.com/shindongyoon-siwon/clipcatch/releases/tag/v99.0.0",
                    }
                route.fulfill(json={"ok": True})
            else:
                route.fulfill(status=400, json={"error": "This test must not install or cancel work."})
        else:
            route.fulfill(status=404, body="Not found")


def check_request(request):
    return urlparse(request.url).path == "/api/check-app-update" and request.method == "POST"


with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=os.environ.get("CLIPCATCH_CHROMIUM") or shutil.which("chromium"), headless=True)
    context = browser.new_context(viewport={"width": 1200, "height": 900})
    api = LocalAPI()
    context.route("http://127.0.0.1:8765/**", api.serve)
    errors = []
    page = context.new_page()
    page.on("pageerror", lambda error: errors.append(str(error)))

    # Opening the page itself performs a quiet fresh check, with no button click.
    with page.expect_request(check_request):
        page.goto("http://127.0.0.1:8765/")
    page.wait_for_timeout(250)
    expect(page.locator("#downloadBtn")).to_be_enabled()
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    assert len(api.checks) == 1
    assert api.checks[0] == {"automatic": True}

    # A burst of return-to-page events produces one check and a dialog from regular polling.
    api.release_available = True
    before = len(api.checks)
    with page.expect_request(check_request):
        page.evaluate("""() => {
            window.dispatchEvent(new Event('focus'));
            window.dispatchEvent(new PageTransitionEvent('pageshow', {persisted:true}));
            document.dispatchEvent(new Event('visibilitychange'));
        }""")
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()
    expect(page.locator("#requiredUpdateInstall")).to_be_enabled()
    page.wait_for_timeout(250)
    assert len(api.checks) == before + 1, "one event burst should produce one automatic request"

    # Hidden tabs do not check; becoming visible discovers the new release.
    api.current()
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    page.evaluate("""() => {
        window.testVisibility='hidden';
        Object.defineProperty(document,'visibilityState',{configurable:true,get:()=>window.testVisibility});
        document.dispatchEvent(new Event('visibilitychange'));
    }""")
    before = len(api.checks)
    page.wait_for_timeout(250)
    assert len(api.checks) == before
    api.release_available = True
    with page.expect_request(check_request):
        page.evaluate("() => {window.testVisibility='visible';document.dispatchEvent(new Event('visibilitychange'));}")
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()

    # Restoring a page has an explicit fresh check even without a focus event.
    api.current()
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    api.release_available = True
    with page.expect_request(check_request):
        page.evaluate("() => window.dispatchEvent(new PageTransitionEvent('pageshow',{persisted:true}))")
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()

    # A failed automatic request leaves normal work available; going online retries.
    api.current()
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    api.offline = True
    with page.expect_request(check_request):
        page.evaluate("() => window.dispatchEvent(new Event('focus'))")
    page.wait_for_timeout(250)
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    expect(page.locator("#downloadBtn")).to_be_enabled()
    api.offline = False
    api.release_available = True
    with page.expect_request(check_request):
        page.evaluate("() => window.dispatchEvent(new Event('online'))")
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()

    # Automatic checks must not interfere with an update already being downloaded/installed.
    for status, label in [("downloading", "업데이트 다운로드"), ("installing", "교체 및 재시작 중")]:
        api.state["app_update"] = {"status": status, "latest": "99.0.0", "percent": 40}
        expect(page.locator("#requiredUpdateInstall")).to_contain_text(label)
        before = len(api.checks)
        page.evaluate("() => {window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('online'));}")
        page.wait_for_timeout(250)
        assert len(api.checks) == before
        expect(page.locator("#requiredUpdateInstall")).to_be_disabled()

    # Existing media work continues; only installation waits for its completion.
    api.current()
    expect(page.locator("#requiredUpdateDialog")).not_to_be_visible()
    api.state["jobs"] = [{"id": "running", "title": "진행 중 영상", "host": "x.com", "url": "https://x.com/example/status/123456", "status": "downloading", "percent": 35}]
    api.state["active_tasks"] = 1
    api.release_available = True
    with page.expect_request(check_request):
        page.evaluate("() => window.dispatchEvent(new Event('focus'))")
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()
    expect(page.locator("#requiredUpdateInstall")).to_be_disabled()
    expect(page.locator(".update-wall .live-job")).to_contain_text("진행 중 영상")
    api.state["jobs"] = []
    api.state["active_tasks"] = 0
    expect(page.locator("#requiredUpdateInstall")).to_be_enabled()

    # A newly opened tab discovers the release on its own initial load.
    api.current()
    api.release_available = True
    second = context.new_page()
    second.on("pageerror", lambda error: errors.append(str(error)))
    with second.expect_request(check_request):
        second.goto("http://127.0.0.1:8765/")
    expect(second.locator("#requiredUpdateDialog")).to_be_visible()
    expect(second.locator("#requiredUpdateInstall")).to_be_enabled()

    assert all(payload == {"automatic": True} for payload in api.checks)
    assert not [path for path, _ in api.requests if path in {"/api/install-app-update", "/api/cancel-download"}]
    assert not errors, errors
    print(json.dumps({"page_load_check": True, "event_coalescing": True, "visible_return": True,
                      "page_restore": True, "offline_recovery": True, "new_tab": True,
                      "automatic_dialog_from_polling": True, "active_work_preserved": True,
                      "no_unattended_install": True, "update_in_progress_guard": True,
                      "javascript_errors": errors}))
    browser.close()
