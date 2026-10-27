"""Real Shell helper checks; record foreground observation, never invent it."""
import ctypes,json,os,subprocess,sys,tempfile,time
from pathlib import Path
if os.name!='nt':raise SystemExit('Windows required')
exe,out=map(lambda s:Path(s).resolve(),sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
root=Path(tempfile.mkdtemp(prefix='clipcatch-folder198-'))
folder=root/('ClipCatch 한글 & folder (1)');folder.mkdir();original=folder/'preserve.txt';original.write_text('unchanged')
def request(path):
 p=subprocess.run([str(exe),'--clipcatch-folder-focus',str(path)],capture_output=True,encoding='utf-8',check=True,timeout=10)
 return json.loads(p.stdout)
first=request(folder);assert not first.get('error'),first;assert first['result']['requested'],first
time.sleep(.4)
second=request(folder);assert not second.get('error'),second;assert second['result']['requested'],second
for data in (first,second):
 r=data['result'];assert isinstance(r['foreground'],bool),r
 if not r['foreground']:assert r.get('focus_reason')in ('user_switched','focus_denied','window_changed','window_not_found','cancelled','unverified'),r
bad=request(original);assert bad.get('error')and not bad['result']['requested'],bad
assert original.read_text()=='unchanged'
result={'first':first,'second':second,'file_rejected':True,'original_preserved':True,'real_windows_helper':True,'user_desktop_environment_tested':False}
(out/'folder-foreground-checks.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8');print(json.dumps(result,ensure_ascii=False))
