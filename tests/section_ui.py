"""Real browser DOM and click paths, mocked metadata only (not a live-site test)."""
import json, os, shutil
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright, expect
R=Path(__file__).resolve().parents[1]; W=R/'src/web'
html=(W/'index.html').read_text().replace('__APP_VERSION__','2.1.0-test.1')
html=html.replace('</body>',''.join('<script src="/'+s+'"></script>' for s in ['premiere.js','quality.js','workspace-tabs.js','startup-update.js','sections.js'])+'</body>',1)
state={'version':'2.1.0-test.1','channel':'section-test','phase':'ready','jobs':[],'output_mode':'premiere','app_update':{'status':'current'},'diagnostics':{'checks':[]},'engine':{'channel':'stable'},'save_dir':'section-test'}
calls=[]; unsupported=False

def serve(route):
 global unsupported
 path=urlparse(route.request.url).path
 if path=='/':route.fulfill(content_type='text/html',body=html)
 elif path.endswith('.js') and (W/path[1:]).is_file():route.fulfill(content_type='application/javascript',body=(W/path[1:]).read_text())
 elif path=='/api/state':route.fulfill(json=state)
 elif path=='/api/browser-link/status':route.fulfill(json={'paired':False})
 elif path=='/api/quality/status':route.fulfill(json={'engine_ready':False})
 elif path=='/api/clip-preview':
  v=route.request.post_data_json;calls.append((path,v));q=v.get('quality') or 1080
  route.fulfill(json={'token':'metadata-plan','duration':120,'qualities':[1080,720,480],'quality':q,'partial':'unsupported' if unsupported else 'supported','notice':'Partial source confirmed' if not unsupported else 'Partial download unsupported'})
 elif route.request.method=='POST':
  v=route.request.post_data_json;calls.append((path,v));route.fulfill(json={'ok':True,'count':1,'job_ids':['section-example']})
 else:route.fulfill(status=404,body='not found')
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get('CLIPCATCH_CHROMIUM') or shutil.which('chromium'),headless=True)
 page=browser.new_page(viewport={'width':1200,'height':1000});errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 if os.environ.get('CLIPCATCH_UI_INLINE')=='1':
  def mocked_reply(url,body):
   global unsupported
   path=urlparse(url).path
   if path=='/api/state':return state
   if path=='/api/browser-link/status':return {'paired':False}
   if path=='/api/quality/status':return {'engine_ready':False}
   if path=='/api/clip-preview':
    calls.append((path,body));return {'token':'metadata-plan','duration':120,'qualities':[1080,720,480],'quality':body.get('quality') or 1080,'partial':'unsupported' if unsupported else 'supported','notice':'Partial download unsupported' if unsupported else 'Partial source confirmed'}
   if body is not None:calls.append((path,body))
   return {'ok':True,'count':1,'job_ids':['section-example']}
  page.expose_function('__testReply',mocked_reply)
  page.evaluate("window.fetch=async (url,opt={})=>new Response(JSON.stringify(await window.__testReply(String(url),opt.body?JSON.parse(opt.body):null)),{status:200,headers:{'Content-Type':'application/json'}})")
  inline=html
  for script in ['premiere.js','quality.js','workspace-tabs.js','startup-update.js','sections.js']:
   inline=inline.replace('<script src="/'+script+'"></script>','<script>'+ (W/script).read_text()+'</script>')
  page.set_content(inline)
 else:
  page.route('http://127.0.0.1:18767/**',serve);page.goto('http://127.0.0.1:18767/')
 page.locator('#urlInput').fill('https://www.youtube.com/watch?v=BaW_jenozKc')
 expect(page.locator('#clipInfo')).to_contain_text('confirmed',timeout=5000)
 assert any(path=='/api/clip-preview' for path,_ in calls)
 assert not any(path in ('/api/download','/api/clip-queue') for path,_ in calls)
 page.locator('#clipQuality').select_option('720');expect(page.locator('#clipPartial')).to_be_enabled()
 page.locator('#clipPartial').check();page.locator('#clipStart').fill('01:00.250');page.locator('#clipEnd').fill('01:02.750')
 page.locator('#downloadBtn').click();page.wait_for_timeout(100)
 queued=[body for path,body in calls if path=='/api/clip-queue'];assert len(queued)==1, calls
 assert queued[0]['start']=='01:00.250' and queued[0]['end']=='01:02.750' and queued[0]['partial'] is True and queued[0]['quality']==720,queued
 assert not any(path=='/api/download' for path,_ in calls),calls
 unsupported=True;page.locator('#clipQuality').select_option('480');expect(page.locator('#clipInfo')).to_contain_text('unsupported')
 page.locator('#downloadBtn').click();page.wait_for_timeout(50)
 assert sum(path=='/api/clip-queue' for path,_ in calls)==1
 assert not any(path=='/api/download' for path,_ in calls)
 expect(page.locator('#urlInput')).to_have_value('https://www.youtube.com/watch?v=BaW_jenozKc')
 unsupported=False;page.locator('#clipRefresh').click();expect(page.locator('#clipInfo')).to_contain_text('confirmed')
 page.locator('#clipEnd').fill('01:00.000');page.locator('#downloadBtn').click();page.wait_for_timeout(50)
 assert sum(path=='/api/clip-queue' for path,_ in calls)==1
 page.locator('#clipEnd').fill('01:02.750');page.locator('#urlInput').press('Control+Enter');page.wait_for_timeout(100)
 assert sum(path=='/api/clip-queue' for path,_ in calls)==2
 out=Path(os.environ.get('CLIPCATCH_QA_OUTPUT',R/'section-evidence'));out.mkdir(exist_ok=True,parents=True)
 page.screenshot(path=str(out/'section-controls.png'),full_page=True)
 page.set_viewport_size({'width':550,'height':950});assert page.evaluate('document.documentElement.scrollWidth<=window.innerWidth'), 'horizontal overflow'
 page.screenshot(path=str(out/'section-controls-small.png'),full_page=True)
 assert not errors,errors
 (out/'section-ui.json').write_text(json.dumps({'status':'passed','scope':'actual Chromium UI, mocked metadata','checks':['auto-preview','quality-recheck','exact-times','no-full-fallback','invalid-times','keyboard','retained-link','small-viewport']},indent=2))
 browser.close()
print('section UI checks passed')
