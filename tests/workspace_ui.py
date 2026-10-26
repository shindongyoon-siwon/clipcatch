"""Offline checks for the actual workspace DOM; no downloads or Windows app execution."""
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
html = (WEB / "index.html").read_text()
html = html.replace("</body>", "".join(f'<script src="/{name}"></script>' for name in SCRIPTS) + "</body>", 1)
html = html.replace("__APP_VERSION__", VERSION)

state = {
    "version": VERSION, "channel": "stable", "phase": "ready", "jobs": [],
    "save_dir": "C:\\Users\\Designer\\Downloads\\ClipCatch", "output_mode": "premiere",
    "app_update": {"status": "current"}, "update_required": False, "update_checking": False,
    "engine": {"channel": "stable", "message": "사용 가능"}, "diagnostics": {"checks": []},
}
requests = []


def serve(route):
    path = urlparse(route.request.url).path
    if path == "/":
        route.fulfill(status=200, content_type="text/html; charset=utf-8", body=html)
    elif path.endswith(".js") and (WEB / path[1:]).is_file():
        route.fulfill(status=200, content_type="application/javascript; charset=utf-8", body=(WEB / path[1:]).read_text())
    elif path == "/api/state":
        route.fulfill(json=state)
    elif path == "/api/browser-link/status":
        route.fulfill(json={"paired": True})
    elif path == "/api/quality/status":
        route.fulfill(json={"engine_ready": False, "engine_bundled": False})
    elif route.request.method == "POST":
        payload = route.request.post_data_json
        requests.append((path, payload))
        if path == "/api/premiere/settings":
            state["output_mode"] = payload["mode"]
        response = {"ok": True, "count": 1}
        if path == "/api/browser-link/install":
            response = {"opened": True, "path": "C:\\ClipCatch\\browser-extension"}
        route.fulfill(json=response)
    else:
        route.fulfill(status=404, body="Not found")


with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=os.environ.get("CLIPCATCH_CHROMIUM") or shutil.which("chromium"), headless=True)
    page = browser.new_page(viewport={"width": 1200, "height": 900})
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.add_init_script("Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>{window.__copied=text;}}});")
    page.route("http://127.0.0.1:8765/**", serve)
    page.goto("http://127.0.0.1:8765/")
    expect(page.locator("#downloadBtn")).to_be_visible()
    expect(page.locator("#navQuality")).to_be_visible()
    expect(page.locator("#navExtension")).to_be_visible()
    expect(page.locator("#buildIdentity")).to_be_hidden()
    expect(page.locator("#shareStable")).to_be_hidden()
    expect(page.locator("#settingsPanel")).not_to_have_attribute("open", "")
    expect(page.locator(".hero,.perks,.safe-note,#trialBanner")).to_have_count(0)
    assert "운영판" not in page.locator("body").inner_text()

    page.locator("#urlInput").fill("https://x.com/example/status/123456")
    page.locator("#downloadBtn").click()
    expect(page.locator("#urlInput")).to_have_value("")
    assert sum(path == "/api/download" for path, _ in requests) == 1
    page.locator("#localConversionTools>summary").click()
    expect(page.locator("#convertLocalFiles")).to_be_visible()
    expect(page.locator("#repairPremiere")).to_be_visible()
    page.locator("#localConversionTools>summary").click()

    page.locator("#navQuality").click()
    expect(page.locator("#qualityPanel")).to_be_visible()
    expect(page.locator("#downloadPanel")).to_be_hidden()
    page.locator("#navExtension").click()
    expect(page.locator("#browserLinkPanel")).to_be_visible()
    page.locator("#installBrowserExtension").click()
    expect(page.locator("#browserLinkStatus")).to_contain_text("설치 폴더를 열었습니다")
    assert "TEST" not in page.locator("#browserLinkPanel").inner_text()
    page.locator("#navDownload").click()

    page.locator("#settingsPanel>summary").click()
    page.locator("#shareStable").click()
    assert page.evaluate("window.__copied") == "https://github.com/shindongyoon-siwon/clipcatch/releases/latest/download/ClipCatch.exe"
    page.locator("#checkAppUpdate").click()
    expect(page.locator("#updateFeedback")).to_be_visible()
    assert sum(path == "/api/check-app-update" for path, _ in requests) == 1
    page.locator("#helpPanel>summary").click()
    page.locator("#updateEngineStable").click()
    assert sum(path == "/api/update-tool" for path, _ in requests) == 1
    page.locator("#settingsPanel>summary").click()

    # One failed media item used to be hidden because the renderer required more than one.
    state["jobs"] = [{
        "id": "failed-one", "title": "한글 제목 영상", "url": "https://x.com/example/status/123456",
        "host": "x.com", "status": "error", "error_code": "CC-421", "error_title": "영상 확인 필요",
        "detail": "완료 파일을 확인하지 못했습니다.", "time": "2026-10-26 16:30", "task": "download",
        "media": [{"original": "C:\\ClipCatch\\한글 제목.mp4", "error_code": "CC-420", "error": "영상 검사에 실패했습니다. <img src=x onerror=alert(1)>"}],
    }]
    page.evaluate("st=>render(st)", state)
    expect(page.locator(".post-files")).to_have_attribute("open", "")
    expect(page.locator(".post-file.error")).to_contain_text("CC-420")
    expect(page.locator(".post-file.error")).to_contain_text("영상 검사에 실패했습니다")
    expect(page.locator(".post-files img")).to_have_count(0)
    page.get_by_role("button", name="문제 해결 열기", exact=True).click()
    expect(page.locator("#settingsPanel")).to_have_attribute("open", "")
    expect(page.locator("#runDiagnostics")).to_be_visible()
    page.locator("#settingsPanel>summary").click()

    state["jobs"][0].update({"host": "reddit.com", "url": "https://reddit.com/r/test/comments/abc123/example", "auth_required": True})
    page.evaluate("st=>render(st)", state)
    page.get_by_role("button", name="레딧 로그인 연결하기", exact=True).click()
    expect(page.locator("#settingsPanel")).to_have_attribute("open", "")
    expect(page.locator("#redditCookieFile")).to_be_visible()
    page.locator("#settingsPanel>summary").click()

    # The required-update modal must remain reachable even with settings collapsed.
    state["update_required"] = True
    state["app_update"] = {"status": "available", "latest": "99.0.0"}
    page.evaluate("st=>render(st)", state)
    expect(page.locator("#requiredUpdateDialog")).to_be_visible()
    expect(page.locator("#requiredUpdateInstall")).to_be_enabled()
    state["update_required"] = False
    state["app_update"] = {"status": "current"}
    state["channel"] = "test"
    page.evaluate("st=>render(st)", state)
    expect(page.locator("#buildIdentity")).to_be_visible()
    expect(page.locator("#buildIdentity")).to_have_text("TEST2 테스트판")
    state["channel"] = "stable"
    state["jobs"] = []
    page.evaluate("st=>render(st)", state)

    output = Path(os.environ.get("CLIPCATCH_QA_OUTPUT", str(ROOT / "qa-output")))
    output.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=str(output / "workspace-clean-desktop.png"), full_page=True)
    page.set_viewport_size({"width": 390, "height": 844})
    assert page.evaluate("document.documentElement.scrollWidth<=innerWidth"), "narrow viewport overflow"
    page.screenshot(path=str(output / "workspace-clean-narrow.png"), full_page=True)
    assert not errors, errors
    print(json.dumps({"workspace_controls": True, "settings_actions": True, "single_file_error": True, "nested_error_help": True, "required_update": True, "test_channel_identity": True, "narrow_layout": True, "javascript_errors": errors}))
    browser.close()
