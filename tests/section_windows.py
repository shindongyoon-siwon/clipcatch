"""Native candidate EXE identity/startup/early unsupported-result and data isolation.
This does not call a fixture result proof of real social-platform downloads.
"""
import hashlib,json,os,subprocess,sys,tempfile,time,urllib.request
from pathlib import Path
exe=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
base='http://127.0.0.1:18767'
def request(path,body=None):
 data=None if body is None else json.dumps(body).encode()
 req=urllib.request.Request(base+path,data=data,headers={'Content-Type':'application/json','Origin':base})
 with urllib.request.urlopen(req,timeout=18) as res:return json.load(res)
with tempfile.TemporaryDirectory(prefix='clipcatch-section-native-') as root:
 root=Path(root);identity=root/'build-info.json'
 r=subprocess.run([str(exe),'--build-info',str(identity)],timeout=12)
 assert r.returncode==0
 info=json.loads(identity.read_text());assert info['app']=='clipcatch-section-test' and info['channel']=='section-test' and info['version']=='2.1.0-test.2',info
 assert info.get('share_url')=='', info
 (out/'candidate-build-info.json').write_text(json.dumps(info,indent=2))
 env=os.environ.copy();env.update(LOCALAPPDATA=str(root/'local'),USERPROFILE=str(root/'home'),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_SECTION_SAVEDIR=str(root/'clips'))
 protected=[]
 for name in ['ClipCatch','ClipCatch-Test2']:
  p=root/'local'/name/'history.json';p.parent.mkdir(parents=True);p.write_bytes(b'original stable history marker');protected.append(p)
 proc=subprocess.Popen([str(exe)],env=env)
 try:
  health=None
  for _ in range(60):
   try:health=request('/api/health');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('candidate exited before health')
    time.sleep(.3)
  assert health and health['app']=='clipcatch-section-test' and health['channel']=='section-test',health
  started=time.monotonic()
  result=request('/api/clip-preview',{'url':'https://www.reddit.com/r/catsistanbul/comments/1x0k17b/the_new_employee_has_an_idea/','quality':0})
  elapsed=time.monotonic()-started
  assert result['partial']=='unsupported' and elapsed<3,result
  assert request('/api/state')['jobs']==[], 'preview queued an unwanted download'
  assert all(p.read_bytes()==b'original stable history marker' for p in protected)
  assert not (root/'local'/'Programs'/'ClipCatch'/'ClipCatch.exe').exists()
  assert not (root/'local'/'Programs'/'ClipCatch-Test2'/'ClipCatch-Test2.exe').exists()
  (out/'native-section.json').write_text(json.dumps({'status':'passed','app':health,'preview_seconds':elapsed,'preview_status':result['partial'],'production_and_legacy_test_data_unchanged':True,'scope':'actual Windows EXE; Reddit page guidance only, no live download success claim'},indent=2))
 finally:
  try:request('/api/stop',{})
  except Exception:pass
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
(out/'SHA256SUMS').write_text(hashlib.sha256(exe.read_bytes()).hexdigest()+'  '+exe.name+'\n')
print('Native section candidate verification passed')
