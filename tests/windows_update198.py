"""Native regression: running 1.9.7, locked legacy backup and two helpers.
No credentials, developer overrides or production profile are used.
"""
import ctypes,hashlib,json,os,shutil,subprocess,sys,tempfile,time,urllib.request
from pathlib import Path
if os.name!='nt':raise SystemExit('Windows required')
exe,out=map(lambda p:Path(p).resolve(),sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
version=(Path(__file__).resolve().parents[1]/'VERSION').read_text().strip()
root=Path(tempfile.mkdtemp(prefix='clipcatch-update197-'));base='http://127.0.0.1:8765'
old_version=sys.argv[3] if len(sys.argv)>3 else '1.9.7'
hashes={'1.9.7':'c59d40e27bc508e672a546371982e6ddb2a3733c46dca67b037cd6755d0fe843','1.9.8':'8477eaa0350d6efd0531a3d0e9dd9b524aa9f5cefd0c0e044927de997e571f40'}
old=root/'old.exe';url=f'https://github.com/shindongyoon-siwon/clipcatch/releases/download/v{old_version}/ClipCatch.exe'
expected=hashes[old_version]
for attempt in range(3):
 try:
  with urllib.request.urlopen(url,timeout=60) as r:old.write_bytes(r.read())
  assert hashlib.sha256(old.read_bytes()).hexdigest()==expected,'Unverified old fixture'
  break
 except Exception:
  if attempt==2:raise
  time.sleep(1)
env=os.environ.copy();env.update(LOCALAPPDATA=str(root/'local'),APPDATA=str(root/'roaming'),USERPROFILE=str(root/'profile'),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_PORTABLE='0')
for k in ('LOCALAPPDATA','APPDATA','USERPROFILE'):Path(env[k]).mkdir(parents=True)
(Path(env['USERPROFILE'])/'Desktop').mkdir()
data=Path(env['LOCALAPPDATA'])/'ClipCatch';tools=data/'tools';tools.mkdir(parents=True)
fixture=root/'tool.go';fixture.write_text('package main\nimport "fmt"\nfunc main(){fmt.Println("2026.10.27")}\n')
subprocess.run(['go','build','-o',str(root/'tool.exe'),str(fixture)],check=True,timeout=60)
for n in ('yt-dlp.exe','ffmpeg.exe','ffprobe.exe'):shutil.copy2(root/'tool.exe',tools/n)
target=Path(env['LOCALAPPDATA'])/'Programs/ClipCatch/ClipCatch.exe';target.parent.mkdir(parents=True);shutil.copy2(old,target)
saved=Path(env['USERPROFILE'])/'Downloads/ClipCatch/keep-original.bin';saved.parent.mkdir(parents=True);saved.write_bytes(b'preserve-original')
(data/'history.json').write_text(json.dumps([{'id':'history-197','status':'done','title':'keep history','file':str(saved),'url':'https://x.com/example/status/123456'}]))
(data/'output-preference.json').write_text('{"mode":"original"}')
backup=Path(str(target)+'.backup');backup.write_bytes(b'locked older backup')
create=ctypes.WinDLL('kernel32',use_last_error=True).CreateFileW
create.argtypes=[ctypes.c_wchar_p,ctypes.c_uint32,ctypes.c_uint32,ctypes.c_void_p,ctypes.c_uint32,ctypes.c_uint32,ctypes.c_void_p];create.restype=ctypes.c_void_p
close=ctypes.WinDLL('kernel32',use_last_error=True).CloseHandle;close.argtypes=[ctypes.c_void_p]
lock=create(str(backup),0x80000000,1,None,3,0x80,None)
assert lock not in (None,ctypes.c_void_p(-1).value),'Could not set up actual sharing violation'
log=(out/'native-update197.log').open('w',encoding='utf-8');procs=[]
def get(path):
 with urllib.request.urlopen(base+path,timeout=2)as r:return json.load(r)
def wait(fn,timeout=35):
 until=time.monotonic()+timeout
 while time.monotonic()<until:
  try:
   x=fn()
   if x:return x
  except (OSError,ValueError):pass
  time.sleep(.1)
 raise AssertionError('Native update condition timed out')
def stop():
 req=urllib.request.Request(base+'/api/stop',data=b'{}',headers={'Origin':base,'Content-Type':'application/json'})
 try:
  with urllib.request.urlopen(req,timeout=3):pass
 except OSError:pass
try:
 running=subprocess.Popen([str(target)],env=env,stdout=log,stderr=log);procs.append(running)
 wait(lambda:get('/api/health')['version']==old_version);wait(lambda:get('/api/state')['phase']=='ready')
 args=[str(exe),'--clipcatch-apply-update',str(target),version]
 helpers=[subprocess.Popen(args,env=env,stdout=log,stderr=log)for _ in range(2)];procs+=helpers
 for helper in helpers:assert helper.wait(timeout=60)==0
 assert running.wait(timeout=15)==0,'Old instance did not exit'
 wait(lambda:get('/api/health')['version']==version)
 receipt=json.loads((data/'update-receipt.json').read_text());assert receipt['status']=='verified',receipt
 assert hashlib.sha256(target.read_bytes()).hexdigest()==hashlib.sha256(exe.read_bytes()).hexdigest()
 assert backup.read_bytes()==b'locked older backup'
 originals=list(target.parent.glob('ClipCatch.exe.backup-*'));assert any(hashlib.sha256(p.read_bytes()).hexdigest()==expected for p in originals)
 state=get('/api/state');assert state['output_mode']=='original'and any(j['id']=='history-197'for j in state['jobs'])
 assert saved.read_bytes()==b'preserve-original'
 count=len(originals);again=subprocess.Popen(args,env=env,stdout=log,stderr=log);procs.append(again);assert again.wait(timeout=20)==0
 assert len(list(target.parent.glob('ClipCatch.exe.backup-*')))==count
 assert get('/api/health')['version']==version
 (out/'native-update197-checks.json').write_text(json.dumps({'version':version,'actual_old_version':old_version,'actual_old_was_running':True,'graceful_process_exit_confirmed':True,'locked_legacy_backup_preserved':True,'new_previous_binary_preserved':True,'two_concurrent_helpers_safe':True,'repeated_current_helper_no_second_swap':True,'history_preference_original_preserved':True,'real_user_machine_tested':False},indent=2))
 print('Native running-1.9.7, locked-backup, concurrent-helper and preservation checks passed')
finally:
 stop();time.sleep(1)
 for p in procs:
  if p.poll()is None:p.terminate();p.wait(timeout=10)
 close(lock);log.close()
 if (data/'updater.log').exists():shutil.copy2(data/'updater.log',out/'updater197.log')
 if (data/'update-failure.json').exists():shutil.copy2(data/'update-failure.json',out/'update-failure197.json')
