"""Native Windows release checks with real old/new apps in disposable profiles.
Tiny tool stand-ins isolate lifecycle checks; real media tests run separately.
"""
import hashlib,json,os,shutil,subprocess,sys,tempfile,threading,time,urllib.error,urllib.request
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
if os.name!='nt':raise SystemExit('Native Windows required')
exe,old,out=map(lambda p:Path(p).resolve(),sys.argv[1:4]);out.mkdir(parents=True,exist_ok=True)
version=(Path(__file__).resolve().parents[1]/'VERSION').read_text().strip()
root=Path(tempfile.mkdtemp(prefix='clipcatch-release-qa-'));base='http://127.0.0.1:8765';seen=[];servers=[]
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def wait(fn,seconds=45):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  try:
   result=fn()
   if result:return result
  except (OSError,ValueError,urllib.error.URLError):pass
  time.sleep(.2)
 raise AssertionError('Native condition timed out')
def get(path):
 with urllib.request.urlopen(base+path,timeout=3) as r:return json.load(r)
def post(path,data):
 request=urllib.request.Request(base+path,data=json.dumps(data).encode(),headers={'Content-Type':'application/json','Origin':base})
 with urllib.request.urlopen(request,timeout=5) as r:return json.load(r)
def stopped():
 try:get('/api/health');return False
 except OSError:return True
def stop():
 try:post('/api/stop',{})
 except OSError:pass
 wait(stopped,15);time.sleep(.5)
class Other(BaseHTTPRequestHandler):
 def do_GET(self):
  seen.append((self.server.server_port,self.path));self.send_response(200);self.end_headers();self.wfile.write(b'{"app":"other-channel"}')
 def do_POST(self):
  seen.append((self.server.server_port,self.path));self.send_response(403);self.end_headers()
 def log_message(self,*args):pass
for port in (18766,18796):
 s=ThreadingHTTPServer(('127.0.0.1',port),Other);servers.append(s);threading.Thread(target=s.serve_forever,daemon=True).start()
helper=root/'fixture-tool.go';helper.write_text('package main\nimport "fmt"\nfunc main(){fmt.Println("2026.10.27")}\n')
subprocess.run(['go','build','-o',str(root/'fixture-tool.exe'),str(helper)],check=True,timeout=60)
def profile(name):
 home=root/name;env=os.environ.copy();env.update(LOCALAPPDATA=str(home/'local'),APPDATA=str(home/'roaming'),USERPROFILE=str(home/'profile'),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_PORTABLE='0')
 for key in ('LOCALAPPDATA','APPDATA','USERPROFILE'):Path(env[key]).mkdir(parents=True,exist_ok=True)
 (Path(env['USERPROFILE'])/'Desktop').mkdir()
 data=Path(env['LOCALAPPDATA'])/'ClipCatch';tools=data/'tools';tools.mkdir(parents=True)
 for name in ('yt-dlp.exe','ffmpeg.exe','ffprobe.exe'):shutil.copy2(root/'fixture-tool.exe',tools/name)
 paths=[]
 for channel in ('ClipCatch-Dev196','ClipCatch-Test2'):
  for folder in (Path(env['LOCALAPPDATA']),Path(env['LOCALAPPDATA'])/'Programs',Path(env['USERPROFILE'])/'Downloads'):
   f=folder/channel/'preserve.txt';f.parent.mkdir(parents=True,exist_ok=True);f.write_text(channel);paths.append(f)
 return env,data,{str(f):digest(f)for f in paths}
def identity(binary,env,label):
 info=out/(label+'-build-info.json');subprocess.run([str(binary),'--build-info',str(info)],env=env,check=True,timeout=15);d=json.loads(info.read_text())
 assert d['app']=='clipcatch'and d['channel']=='stable'and d['version']==version and d['share_url'].endswith('/latest/download/ClipCatch.exe'),d
 return d
def intact(paths):
 for f,h in paths.items():assert digest(Path(f))==h,f
log=(out/'windows-release-lifecycle.log').open('w',encoding='utf-8');processes=[]
try:
 env,data,pins=profile('upgrade');target=Path(env['LOCALAPPDATA'])/'Programs/ClipCatch/ClipCatch.exe';target.parent.mkdir(parents=True);shutil.copy2(old,target)
 saved=Path(env['USERPROFILE'])/'Downloads/ClipCatch/keep-original.png';saved.parent.mkdir(parents=True);saved.write_bytes(b'preserve-existing-user-file');h=digest(saved)
 (data/'history.json').write_text(json.dumps([{'id':'preserved-history','status':'done','title':'preserved item','file':str(saved),'url':'https://x.com/example/status/123456','time':'2026-10-26 10:00'}]))
 (data/'output-preference.json').write_text('{"mode":"original"}')
 oldproc=subprocess.Popen([str(target)],env=env,stdout=log,stderr=log);processes.append(oldproc)
 wait(lambda:get('/api/health').get('version')=='1.9.5');assert any(j['id']=='preserved-history'for j in get('/api/state')['jobs'])
 # Keep the actual old program RUNNING; the new helper must stop and wait.
 identity(exe,env,'candidate')
 subprocess.run([str(exe),'--clipcatch-apply-update',str(target),version],env=env,stdout=log,stderr=log,timeout=45,check=True)
 oldproc.wait(timeout=15)
 wait(lambda:get('/api/health').get('version')==version)
 receipt=json.loads((data/'update-receipt.json').read_text());assert receipt['status']=='verified'and receipt['version']==version,receipt
 assert digest(target)==digest(exe)and digest(Path(str(target)+'.backup'))==digest(old)
 state=get('/api/state');assert state['channel']=='stable'and not state.get('isolated_development')and state['save_dir']==str(saved.parent),state
 assert any(j['id']=='preserved-history'for j in state['jobs']);assert state['output_mode']=='original';assert digest(saved)==h;intact(pins)
 with urllib.request.urlopen(base+'/',timeout=3) as r:assert b'lifecycle.js'in r.read()
 post('/api/workspace/heartbeat',{'id':'a'*32,'ack':0});assert post('/api/workspace/open',{})['reused']
 subprocess.run([str(target)],env=env,stdout=log,stderr=log,check=True,timeout=15);assert get('/api/health')['version']==version
 assert 'paired'in get('/api/browser-link/status')
 stop();intact(pins)
 env2,data2,pins2=profile('fresh');identity(exe,env2,'fresh')
 fresh=subprocess.Popen([str(exe)],env=env2,stdout=log,stderr=log);processes.append(fresh);fresh.wait(timeout=15)
 wait(lambda:get('/api/health').get('version')==version)
 installed=Path(env2['LOCALAPPDATA'])/'Programs/ClipCatch/ClipCatch.exe';assert digest(installed)==digest(exe)
 desktop=Path(env2['USERPROFILE'])/'Desktop/ClipCatch.lnk';menu=Path(env2['APPDATA'])/'Microsoft/Windows/Start Menu/Programs/ClipCatch.lnk';assert desktop.is_file()and menu.is_file()
 wait(lambda:not get('/api/state').get('update_checking'),45)
 q=get('/api/quality/status');assert q['engine_bundled']
 job=post('/api/quality/install',{'consent':True})['id'];finished=wait(lambda:next((j for j in get('/api/state')['jobs']if j['id']==job and j['status']in ('done','error')),None),60);assert finished['status']=='done',finished
 assert get('/api/quality/status')['engine_ready'];intact(pins2)
 opened=post('/api/open-folder',{});assert opened['ok']and opened['requested']and opened['path']==get('/api/state')['save_dir'],opened
 stop();intact(pins2)
 for envx in (env,env2):
  startup=Path(envx['APPDATA'])/'Microsoft/Windows/Start Menu/Programs/Startup';assert not startup.exists()or not list(startup.iterdir()),'Startup registration'
 assert not seen,seen
 result={'version':version,'stable_identity':True,'fresh_install':True,'upgrade_from_actual_1_9_5':True,'previous_binary_backup':True,'history_and_output_preference_preserved':True,'saved_original_unchanged':True,'test2_dev196_ports_and_files_untouched':True,'shortcut_creation':True,'existing_workspace_reuse':True,'production_extension_status_available':True,'bundled_ai_verified_and_activated':True,'native_folder_request_accepted':True,'folder_foreground_focus_asserted':False,'native_gpu_inference_tested':False,'native_tool_mode':'lifecycle-only stand-ins; real media regression is separate'}
 (out/'windows-release-checks.json').write_text(json.dumps(result,indent=2),encoding='utf-8');print(json.dumps(result))
finally:
 try:stop()
 except Exception:pass
 for p in processes:
  if p.poll()is None:p.terminate();p.wait(timeout=10)
 for s in servers:s.shutdown();s.server_close()
 log.close()

subprocess.run([sys.executable,str(Path(__file__).with_name('windows_update197.py')),str(exe),str(out/'repair197')],check=True,timeout=180)
