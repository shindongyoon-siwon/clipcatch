"""Confirmed foreground, policy denial and unknown results use different feedback."""
import json,os,shutil
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
R=Path(__file__).resolve().parents[1]
with sync_playwright() as p:
 b=p.chromium.launch(executable_path=os.environ.get('CLIPCATCH_CHROMIUM')or shutil.which('chromium'),headless=True)
 page=b.new_page();page.set_content('<div><button id="folderBtn">Folder</button></div>');errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 page.evaluate("""()=>{window.lastState={save_dir:'C:/Downloads/ClipCatch'};window.reply={ok:true,requested:true,foreground:true};window.fetch=async()=>new Response(JSON.stringify(reply),{status:200});}""")
 page.add_script_tag(content=(R/'src/web/folder-open.js').read_text(encoding='utf-8'))
 page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).not_to_contain_text('작업표시줄');expect(page.locator('#folderFeedback')).to_contain_text('열었어')
 page.evaluate("reply={ok:true,requested:true,foreground:false,focus_reason:'focus_denied'}");page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).to_contain_text('작업표시줄');expect(page.locator('#folderFeedback')).to_contain_text('ClipCatch')
 page.evaluate("reply={ok:true,requested:true,foreground:false,focus_reason:'user_switched'}");page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).to_contain_text('다른 작업');expect(page.locator('#folderFeedback')).not_to_contain_text('작업표시줄')
 page.evaluate("reply={ok:true,requested:true}");page.locator('#folderBtn').click();expect(page.locator('#folderFeedback')).to_contain_text('확인하지 못했어');assert not errors,errors
 print(json.dumps({'confirmed_foreground_no_taskbar_prompt':True,'denied_and_unknown_feedback_only':True,'user_switch_respected':True,'errors':errors}));b.close()
