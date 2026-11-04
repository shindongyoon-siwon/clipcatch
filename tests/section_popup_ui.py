"""Actual popup DOM interactions with mocked bridge replies; no live-site claim."""
import json, os, re, shutil
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
R=Path(__file__).resolve().parents[1]
E=R/'src/browser_extension'
calls=[]
unsupported=False

def reply(m):
 calls.append(m)
 if m['kind']=='clip-preview':
  return {'token':'popup-plan','duration':120,'quality':m.get('quality') or 1080,'qualities':[1080,720],'partial':'unsupported' if unsupported else 'supported','notice':'Candidate format unsupported' if unsupported else 'Partial source confirmed'}
 return {'ok':True,'job_ids':['popup-section']}

with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CLIPCATCH_CHROMIUM') or shutil.which('chromium'),headless=True)
 page=browser.new_page(viewport={'width':396,'height':900});errors=[]
 page.on('pageerror',lambda e:errors.append(str(e)))
 page.expose_function('__clipReply',reply)
 html=(E/'popup.html').read_text()
 html=re.sub(r'<script[^>]*src="popup.js"[^>]*></script>','',html)
 html=html.replace('<link rel="stylesheet" href="popup.css">','<style>'+(E/'popup.css').read_text()+'</style>')
 page.set_content(html)
 js=(E/'section-popup.mjs').read_text().replace('export function attachClipPopup','function attachClipPopup',1)
 page.add_script_tag(content=js)
 page.evaluate("""()=>{window.fullRequests=0;document.getElementById('send').disabled=false;document.getElementById('send').addEventListener('click',()=>window.fullRequests++);attachClipPopup({message:m=>window.__clipReply(m),selection:()=>({key:'one'}),notice:()=>{},refresh:()=>{}})}""")
 expect(page.locator('#sectionTimes')).to_be_visible()
 for suffix in ['StartH','StartM','StartS','EndH','EndM','EndS']:expect(page.locator('#section'+suffix)).to_be_disabled()
 expect(page.locator('#sectionInfo')).to_contain_text('confirmed')
 page.locator('#sectionOnly').check()
 page.locator('#sectionStartM').fill('1');page.locator('#sectionStartS').fill('0.250')
 page.locator('#sectionEndM').fill('1');page.locator('#sectionEndS').fill('2.750')
 page.locator('#send').click();page.wait_for_timeout(100)
 q=[v for v in calls if v['kind']=='clip-queue']
 assert len(q)==1 and q[0]['start']=='60.25' and q[0]['end']=='62.75' and q[0]['partial'],q
 assert page.evaluate('window.fullRequests')==0
 page.locator('#sectionEndM').fill('60');page.locator('#sectionSave').click();page.wait_for_timeout(80)
 assert len([v for v in calls if v['kind']=='clip-queue'])==1
 page.locator('#sectionEndM').fill('1')
 unsupported=True;page.locator('#sectionQuality').select_option('720')
 expect(page.locator('#sectionInfo')).to_contain_text('unsupported')
 expect(page.locator('#sectionOnly')).to_be_checked();expect(page.locator('#sectionOnly')).to_be_enabled()
 expect(page.locator('#sectionStartM')).to_be_disabled()
 page.locator('#send').click();page.wait_for_timeout(80)
 assert len([v for v in calls if v['kind']=='clip-queue'])==1
 assert page.evaluate('window.fullRequests')==0
 page.locator('#sectionOnly').uncheck()
 expect(page.locator('#sectionTimes')).to_be_visible()
 expect(page.locator('#sectionOnly')).not_to_be_checked()
 unsupported=False;page.locator('#sectionAgain').click();expect(page.locator('#sectionInfo')).to_contain_text('confirmed');expect(page.locator('#sectionQuality')).to_have_value('720')
 page.locator('#sectionOnly').check();expect(page.locator('#sectionStartM')).to_have_value('1')
 assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
 out=Path(os.environ.get('CLIPCATCH_QA_OUTPUT',R/'section-evidence'));out.mkdir(parents=True,exist_ok=True)
 page.locator('#sectionTools').screenshot(path=str(out/'section-popup-controls.png'))
 (out/'section-popup-ui.json').write_text(json.dumps({'status':'passed','scope':'actual Chromium popup DOM, mocked bridge metadata','checks':['numeric-fields','initially-disabled','under-quality','no-full-fallback','invalid-times','failure-keeps-selection','toggle-off-available','narrow-width']},indent=2))
 assert not errors,errors
 browser.close()
print('section popup UI checks passed')
