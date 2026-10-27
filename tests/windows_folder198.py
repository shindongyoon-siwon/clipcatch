"""Real Shell helper checks with independent Explorer HWND observation."""
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
# Independent observer exists only in QA; the shipped app does not use PowerShell.
env=os.environ.copy();env['CLIPCATCH_QA_FOLDER']=str(folder)
script="$ErrorActionPreference='Stop';$s=New-Object -ComObject Shell.Application;foreach($w in $s.Windows()){try{if($w.Document.Folder.Self.Path -eq $env:CLIPCATCH_QA_FOLDER){[long]$w.HWND;break}}catch{}}"
p=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',script],env=env,capture_output=True,text=True,check=True,timeout=15)
hwnd=int(p.stdout.strip());assert hwnd>0,'Actual Explorer folder window was not found'
u=ctypes.WinDLL('user32',use_last_error=True)
u.IsWindow.argtypes=[ctypes.c_void_p];u.IsWindow.restype=ctypes.c_int
u.IsIconic.argtypes=[ctypes.c_void_p];u.IsIconic.restype=ctypes.c_int
u.ShowWindowAsync.argtypes=[ctypes.c_void_p,ctypes.c_int];u.ShowWindowAsync.restype=ctypes.c_int
u.GetForegroundWindow.restype=ctypes.c_void_p
assert u.IsWindow(hwnd),'Explorer window disappeared'
u.ShowWindowAsync(hwnd,6)
end=time.monotonic()+3
while not u.IsIconic(hwnd)and time.monotonic()<end:time.sleep(.05)
assert u.IsIconic(hwnd),'Minimized fixture did not minimize'
third=request(folder);assert not third.get('error')and third['result']['requested'],third
end=time.monotonic()+3
while u.IsIconic(hwnd)and time.monotonic()<end:time.sleep(.05)
assert not u.IsIconic(hwnd),'Explorer folder stayed minimized'
observed=u.GetForegroundWindow()==hwnd
if third['result']['foreground']:assert observed,'Helper claimed foreground without independent confirmation'
for data in (first,second,third):
 r=data['result'];assert isinstance(r['foreground'],bool),r
 if not r['foreground']:assert r.get('focus_reason')in ('user_switched','focus_denied','window_changed','window_not_found','cancelled','unverified'),r
bad=request(original);assert bad.get('error')and not bad['result']['requested'],bad
assert original.read_text()=='unchanged'
result={'first':first,'second':second,'after_minimized':third,'actual_minimized_window_restored':True,'independent_foreground_observed':observed,'file_rejected':True,'original_preserved':True,'real_windows_helper':True,'user_desktop_environment_tested':False}
(out/'folder-foreground-checks.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8');print(json.dumps(result,ensure_ascii=False))
