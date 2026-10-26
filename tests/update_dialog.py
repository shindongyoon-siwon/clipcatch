"""Offline DOM checks for the real mandatory-update UI (not Windows install QA)."""
import json, os, shutil
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
ROOT=Path(__file__).resolve().parents[1]
with sync_playwright() as p:
    browser=p.chromium.launch(executable_path=os.environ.get('CLIPCATCH_CHROMIUM') or shutil.which('chromium'),headless=True)
    page=browser.new_page(viewport={"width":1200,"height":850});errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.set_content('<html><head><meta charset="utf-8"></head><body style="background:#101722;color:white;font-family:Arial"><button id="quitBtn">Exit</button><main>ClipCatch</main></body></html>')
    page.add_script_tag(content="const $=id=>document.getElementById(id);const uiVersion='1.9.2';let lastState=null;let render=st=>{lastState=st};let post=async()=>({ok:true});let poll=async()=>{};")
    page.add_script_tag(content=(ROOT/'src/web/startup-update.js').read_text())
    st={"version":"1.9.1","channel":"stable","update_required":True,"update_checking":False,"active_tasks":0,"jobs":[],"app_update":{"status":"available","latest":"1.9.2","notes":"1. Download improvements\n2. Required update before new work\n3. <img src=x onerror=alert(1)> is text only","release_url":"https://github.com/shindongyoon-siwon/clipcatch/releases/tag/v1.9.2"}}
    page.evaluate('(st)=>render(st)',st)
    expect(page.locator('#requiredUpdateDialog')).to_be_visible();expect(page.locator('#requiredUpdateInstall')).to_be_enabled()
    expect(page.locator('#updateReleaseNotes')).to_contain_text('<img src=x')
    expect(page.locator('#updateReleaseNotes img')).to_have_count(0)
    page.keyboard.press('Escape');expect(page.locator('#requiredUpdateDialog')).to_be_visible()
    st['active_tasks']=1;page.evaluate('(st)=>render(st)',st);expect(page.locator('#requiredUpdateInstall')).to_be_disabled()
    st['active_tasks']=0;st['app_update']['status']='error';st['app_update']['detail']='Network unavailable';page.evaluate('(st)=>render(st)',st)
    expect(page.locator('#requiredUpdateDialog')).to_be_visible();expect(page.locator('#requiredUpdateInstall')).to_be_enabled()
    st['app_update']['release_url']='javascript:alert(1)';page.evaluate('(st)=>render(st)',st);expect(page.locator('.update-notes a')).to_be_hidden()
    st['app_update']['release_url']='https://github.com/shindongyoon-siwon/clipcatch/releases/tag/v1.9.2';st['app_update']['status']='available';page.evaluate('(st)=>render(st)',st)
    out=ROOT/'qa-output';out.mkdir(exist_ok=True);page.screenshot(path=str(out/'mandatory-update-desktop.png'))
    page.set_viewport_size({"width":500,"height":780});page.screenshot(path=str(out/'mandatory-update-narrow.png'))
    assert page.evaluate('document.documentElement.scrollWidth<=innerWidth'), 'page overflow'
    st['update_required']=False;st['update_checking']=True;st['app_update']={'status':'checking'};page.evaluate('(st)=>render(st)',st)
    expect(page.locator('#requiredUpdateDialog')).to_be_visible();expect(page.locator('#requiredUpdateInstall')).to_be_disabled()
    st['update_checking']=False;st['app_update']={'status':'current'};page.evaluate('(st)=>render(st)',st);expect(page.locator('#requiredUpdateDialog')).not_to_be_visible()
    assert not errors,errors
    print(json.dumps({'mandatory_dialog':True,'escape_blocked':True,'safe_notes':True,'active_job_guard':True,'error_keeps_known_update_gate':True,'startup_check_guard':True,'unsafe_link_blocked':True,'narrow_layout':True,'javascript_errors':errors}))
    browser.close()
