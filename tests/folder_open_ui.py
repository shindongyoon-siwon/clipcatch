"""Test the actual folder button code without claiming OS window focus."""
import json,os,shutil
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
root=Path(__file__).resolve().parents[1]
with sync_playwright() as p:
 b=p.chromium.launch(executable_path=os.environ.get('CLIPCATCH_CHROMIUM')or shutil.which('chromium'),headless=True)
 page=b.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 page.set_content('<div><button id="folderBtn">Open folder</button></div>')
 page.evaluate("""()=>{window.lastState={save_dir:'C:\\Downloads\\ClipCatch'};window.calls=0;window.mode='failure';window.fetch=async()=>{window.calls++;if(mode==='timeout')throw new DOMException('timeout','TimeoutError');if(mode==='pending')return await new Promise(resolve=>window.done=resolve);return new Response(JSON.stringify(mode==='success'?{ok:true,requested:true,path:lastState.save_dir}:{ok:false,error:'shell failed <img src=x onerror=alert(1)>',path:lastState.save_dir}),{status:mode==='success'?200:500,headers:{'Content-Type':'application/json'}});};}""")
 page.add_script_tag(content=(root/'src/web/folder-open.js').read_text(encoding='utf-8'))
 page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).to_contain_text('shell failed');expect(page.locator('#folderFeedback')).to_contain_text('ClipCatch');expect(page.locator('#folderFeedback img')).to_have_count(0);expect(page.locator('#folderBtn')).to_be_enabled()
 page.evaluate("mode='success'");page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).not_to_contain_text('shell failed');expect(page.locator('#folderBtn')).to_be_enabled()
 page.evaluate("mode='timeout'");page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).to_contain_text('ClipCatch');expect(page.locator('#folderBtn')).to_be_enabled()
 page.evaluate("mode='pending';calls=0");page.locator('#folderBtn').click();page.evaluate("document.getElementById('folderBtn').click()");assert page.evaluate('calls')==1
 page.evaluate("done(new Response(JSON.stringify({ok:true,requested:true}),{status:200}))");expect(page.locator('#folderBtn')).to_be_enabled();assert not errors,errors
 print(json.dumps({'folder_api_failure_visible':True,'timeout_recovery':True,'path_fallback':True,'no_html_injection':True,'duplicate_click_blocked':True,'javascript_errors':errors}));b.close()
