"""Native Windows startup/isolation/shutdown and bundled-engine verification, no GPU inference."""
import hashlib,json,os,subprocess,sys,tempfile,threading,time,urllib.error,urllib.request
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
exe=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
if os.name!='nt':raise SystemExit('This test requires native Windows')
root=Path(tempfile.mkdtemp(prefix='clipcatch-dev196-qa-'));env=os.environ.copy();env.update(LOCALAPPDATA=str(root/'local'),APPDATA=str(root/'roaming'),USERPROFILE=str(root/'profile'),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_DEV196_FIXTURE='1')
for k in ['LOCALAPPDATA','APPDATA','USERPROFILE']:Path(env[k]).mkdir(parents=True,exist_ok=True)
sentinels=[]
for name in ['ClipCatch','ClipCatch-Test2']:
 for base in [root/'local',root/'local/Programs',root/'profile/Downloads']:
  p=base/name/'keep.txt';p.parent.mkdir(parents=True,exist_ok=True);p.write_text('do-not-touch-'+name);sentinels.append(p)
before={str(p):hashlib.sha256(p.read_bytes()).hexdigest()for p in sentinels}
seen=[]
class OtherChannel(BaseHTTPRequestHandler):
 def do_GET(self):
  seen.append((self.server.server_port,self.path));self.send_response(200);self.end_headers();self.wfile.write(b'{"app":"other-channel"}')
 def do_POST(self):
  seen.append((self.server.server_port,self.path));self.send_response(403);self.end_headers()
 def log_message(self,*args):pass
servers=[]
for port in [8765,18766]:
 s=ThreadingHTTPServer(('127.0.0.1',port),OtherChannel);servers.append(s);threading.Thread(target=s.serve_forever,daemon=True).start()
base='http://127.0.0.1:18796'
def get(path):
 with urllib.request.urlopen(base+path,timeout=2)as r:return json.load(r)
def post(path,obj):
 req=urllib.request.Request(base+path,data=json.dumps(obj).encode(),headers={'Content-Type':'application/json','Origin':base})
 with urllib.request.urlopen(req,timeout=3)as r:return json.load(r)
def wait_for(fn,timeout=20):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  try:
   r=fn()
   if r:return r
  except (OSError,urllib.error.URLError,ValueError):pass
  time.sleep(.15)
 raise AssertionError('Timed out waiting for native test condition')
log=open(out/'windows-lifecycle.log','w',encoding='utf-8');proc=None
try:
 info=out/'build-info.json';subprocess.run([str(exe),'--build-info',str(info)],env=env,check=True,timeout=10)
 identity=json.loads(info.read_text());assert identity['app']=='clipcatch-dev196'and identity['channel']=='test'and identity['version']=='1.9.6-test.1'and not identity['share_url'],identity
 proc=subprocess.Popen([str(exe)],env=env,stdout=log,stderr=log)
 health=wait_for(lambda:get('/api/health'));assert health['app']=='clipcatch-dev196',health
 state=wait_for(lambda:(s if(s:=get('/api/state'))['phase']=='ready'else None));assert state['isolated_development']and not state['distribution_url'];assert state['save_dir'].endswith('ClipCatch-Dev196')
 with urllib.request.urlopen(base+'/',timeout=3)as r:assert b'lifecycle.js'in r.read()
 assert get('/api/quality/status')['engine_bundled']
 jid=post('/api/quality/install',{'consent':True})['id'];j=wait_for(lambda:next((x for x in get('/api/state')['jobs']if x['id']==jid and x['status']in ['done','error']),None),40);assert j['status']=='done',j
 assert get('/api/quality/status')['engine_ready']
 post('/api/workspace/heartbeat',{'id':'a'*32,'ack':0});assert post('/api/workspace/open',{})['reused']
 subprocess.run([str(exe)],env=env,check=True,timeout=10);assert proc.poll()is None
 try:post('/api/install-app-update',{});raise AssertionError('installer allowed')
 except urllib.error.HTTPError as e:assert e.code==403,e.code
 subprocess.run([str(exe),'--clipcatch-apply-update',str(sentinels[0]),'1.9.6-test.1'],env=env,check=True,timeout=10)
 assert post('/api/stop',{})['stopping'];assert proc.wait(timeout=10)==0
 assert not seen,seen
 for p in sentinels:assert hashlib.sha256(p.read_bytes()).hexdigest()==before[str(p)]
 assert not (root/'local/Programs/ClipCatch-Dev196').exists(),'test installed itself'
 result={'native_windows_start_stop':True,'build_identity':identity,'production_and_old_test_ports_untouched':True,'sentinel_files_unchanged':True,'no_auto_install':True,'app_updater_blocked':True,'bundled_ai_install_and_checksum':True,'gpu_inference_tested':False}
 (out/'native-windows-checks.json').write_text(json.dumps(result,indent=2),encoding='utf-8');print(json.dumps(result))
finally:
 if proc and proc.poll()is None:proc.terminate();proc.wait(timeout=10)
 for s in servers:s.shutdown();s.server_close()
 log.close()
