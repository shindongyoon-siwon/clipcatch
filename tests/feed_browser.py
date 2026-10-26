"""Real Chromium extension, synthetic Reddit DOM and local bridge fixture.
No live site download, account, cookie or Windows security claim.
"""
import json, os, threading, tempfile, shutil
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
ROOT=Path(__file__).resolve().parents[1]
seen=[]
class Server(BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def cors(self):
  self.send_header('Access-Control-Allow-Origin',self.headers.get('Origin','*'))
  self.send_header('Access-Control-Allow-Headers','Content-Type, Authorization')
  self.send_header('Access-Control-Allow-Methods','POST, OPTIONS')
  self.send_header('Access-Control-Allow-Private-Network','true')
 def do_OPTIONS(self):
  self.send_response(204); self.cors(); self.end_headers()
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers.get('Content-Length',0))))
  if self.path=='/bridge/pair': data={'token':'b'*64,'version':'1.9.2','protocol':1,'channel':'stable'}
  elif self.headers.get('Authorization')!='Bearer '+'b'*64:
   self.send_response(401);self.cors();self.end_headers();self.wfile.write(b'{"error":"revoked"}');return
  elif self.path=='/bridge/status':
   data={'app':'clipcatch','channel':'stable','protocol':1,'phase':'ready','update_required':False,'capabilities':['job-status','output-mode'],'jobs':[]}
  else:
   seen.append({'route':self.path,'body':body});data={'ok':True,'id':str(len(seen)),'count':1}
  self.send_response(200);self.cors();self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(data).encode())
server=ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
feed='https://www.reddit.com/r/IllegallySmolCats/'
post1='https://www.reddit.com/r/aww/comments/abc123/first/'
post2='https://www.reddit.com/r/aww/comments/def456/second/'
a='https://v.redd.it/firstmedia123/DASHPlaylist.mpd'
b='https://v.redd.it/secondmedia123/DASHPlaylist.mpd'
style='<style>shreddit-post{display:block;width:700px;height:180px;padding:20px;border:1px solid #aaa}</style>'
def card(id,url,title,media):
 return '<shreddit-post id="t3_'+id+'" permalink="'+url+'" post-title="'+title+'" post-type="video"><h2>'+title+'</h2>'+''.join('<shreddit-player playback-media-json=\''+json.dumps({'dash_url':s})+'\'></shreddit-player>' for s in media)+'</shreddit-post>'
html=style+card('abc123',post1,'Video A',[a])+card('def456',post2,'Video B',[b])
checks=[]
def check(name,value):
 assert value,name
 checks.append(name)
 print('PASS',name,flush=True)
try:
 with tempfile.TemporaryDirectory() as d, sync_playwright() as p:
  ext=ROOT/'src/browser_extension'
  ctx=p.chromium.launch_persistent_context(d,executable_path=os.environ.get('CLIPCATCH_CHROMIUM') or shutil.which('chromium'),channel='chromium',headless=True,ignore_default_args=['--disable-extensions'],args=['--no-sandbox','--disable-extensions-except='+str(ext),'--load-extension='+str(ext)])
  ctx.set_default_timeout(12000)
  worker=ctx.service_workers[0] if ctx.service_workers else ctx.wait_for_event('serviceworker')
  extid=worker.url.split('/')[2]
  page=ctx.new_page();page.route('https://www.reddit.com/**',lambda r:r.fulfill(body=html,content_type='text/html'));page.goto(feed)
  popup=ctx.new_page();errors=[];popup.on('pageerror',lambda e:errors.append(str(e)));popup.goto('chrome-extension://'+extid+'/popup.html')
  result=popup.evaluate('(code)=>chrome.runtime.sendMessage({kind:"pair",value:code})',f'http://127.0.0.1:{server.server_port}#'+'a'*64)
  check('paired',result['ok']);page.bring_to_front()
  result=popup.evaluate('chrome.runtime.sendMessage({kind:"list"})')
  check('feed_two_choices',result['ok'] and len(result['data']['candidates'])==2)
  choices=result['data']['candidates'];check('association',choices[0]['page_url']==post1 and choices[1]['page_url']==post2)
  popup.evaluate("document.getElementById('refresh').click()")
  expect(popup.locator('.video-item')).to_have_count(2)
  check('checkboxes',popup.locator('.video-item input[type=checkbox]').count()==2)
  check('no_silent_bulk_selection',popup.locator('.video-item input:checked').count()==0)
  popup.evaluate("document.getElementById('selectAll').click()")
  check('select_all',popup.locator('.video-item input:checked').count()==2)
  r=popup.evaluate('(keys)=>chrome.runtime.sendMessage({kind:"send-selected",keys})',[c['key'] for c in choices])
  check('two_handoffs',r['ok'] and len(r['data']['results'])==2 and all(x['ok'] for x in r['data']['results']))
  check('correct_streams',[x['body'].get('media_url') for x in seen]==[a,b])
  check('separate_identity',all(x['body'].get('separate') for x in seen))
  check('no_cdn_urls_in_popup',a not in popup.content() and b not in popup.content())
  html=style+card('abc123',post1,'Two videos',[a,b]);page.goto(post1);page.bring_to_front()
  r=popup.evaluate('chrome.runtime.sendMessage({kind:"list"})')
  check('same_post_two_choices',r['ok'] and len(r['data']['candidates'])==2)
  same=r['data']['candidates'];check('distinct_keys',same[0]['key']!=same[1]['key'])
  r=popup.evaluate('(keys)=>chrome.runtime.sendMessage({kind:"send-selected",keys})',[x['key'] for x in same])
  check('same_post_both_sent',r['ok'] and all(x['ok'] for x in r['data']['results']))
  check('forged_denied',not popup.evaluate('chrome.runtime.sendMessage({kind:"send-selected",keys:["forged"]})')['ok'])
  page.goto(feed);page.bring_to_front()
  r=popup.evaluate('(keys)=>chrome.runtime.sendMessage({kind:"send-selected",keys})',[x['key'] for x in same]);check('stale_page_denied',not r['ok'])
  for keys,label in [([], 'empty'),(['x']*11,'oversized'),([same[0]['key']]*2,'repeated')]:
   check(label+'_denied',not popup.evaluate('(keys)=>chrome.runtime.sendMessage({kind:"send-selected",keys})',keys)['ok'])
  html=style+'<h1>No loaded video</h1>';page.reload();page.bring_to_front()
  r=popup.evaluate('chrome.runtime.sendMessage({kind:"list"})');check('empty_feed',r['ok'] and r['data']['candidates']==[])
  check('no_js_errors',not errors)
  ctx.close()
finally:
 server.shutdown()
print(json.dumps({'checks':checks,'live_site_download_tested':False,'windows_security_tested':False},indent=2))
