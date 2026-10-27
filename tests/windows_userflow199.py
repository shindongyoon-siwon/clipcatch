"""Actual Chrome -> shipped HTTP route -> Explorer; optional published updater paths.
Runs in disposable Windows profiles. No mocked HTTP, TLS trust changes, flags to
bypass update verification, or direct apply-update helper in the public cases.
Tiny tools isolate startup only; media regression runs separately.
"""
import hashlib,json,os,queue,shutil,subprocess,sys,tempfile,threading,time,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
if os.name!='nt': raise SystemExit('Windows required')
exe,out=map(lambda p:Path(p).resolve(),sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
public='--public' in sys.argv[3:]
version=(Path(__file__).resolve().parents[1]/'VERSION').read_text().strip()
root=Path(tempfile.mkdtemp(prefix='clipcatch-userflow199-'));base='http://127.0.0.1:8765'
repo='https://github.com/shindongyoon-siwon/clipcatch'
hashes={'1.9.9':'68c61e89bac6ac465822cb7521b9821d49bffcdcc346c88c43cef183a323fc58','1.9.6':'c995502a2942c33ede7f836fe1bb7178e72a4598ca127d29e5c80698fb7bbd58','1.9.8':'8477eaa0350d6efd0531a3d0e9dd9b524aa9f5cefd0c0e044927de997e571f40'}
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def get(path):
 with urllib.request.urlopen(base+path,timeout=3) as r:return json.load(r)
def wait(fn,timeout=60):
 end=time.monotonic()+timeout;last=None
 while time.monotonic()<end:
  try:
   result=fn()
   if result:return result
  except (OSError,ValueError) as e:last=e
  time.sleep(.15)
 raise AssertionError(f'Condition timed out: {last}')
def post(path):
 req=urllib.request.Request(base+path,data=b'{}',headers={'Content-Type':'application/json','Origin':base})
 with urllib.request.urlopen(req,timeout=3) as r:return json.load(r)
def stop():
 try:post('/api/stop')
 except OSError:pass
 def stopped():
  try:get('/api/health');return False
  except OSError:return True
 wait(stopped,15);time.sleep(.4)
def read_line(proc,timeout=20):
 q=queue.Queue();threading.Thread(target=lambda:q.put(proc.stdout.readline()),daemon=True).start()
 line=q.get(timeout=timeout)
 if not line:raise AssertionError('Explorer observer stopped without evidence')
 return line.strip()
def fetch_old(v):
 dest=root/f'old-{v}.exe'
 if not dest.exists():
  for attempt in range(3):
   try:
    with urllib.request.urlopen(f'{repo}/releases/download/v{v}/ClipCatch.exe',timeout=60) as r:dest.write_bytes(r.read())
    assert sha(dest)==hashes[v],'Old executable checksum mismatch';break
   except Exception:
    if attempt==2:raise
    time.sleep(1)
 return dest
fixture=root/'fixture.go';fixture.write_text('package main\nimport "fmt"\nfunc main(){fmt.Println("2026.10.27")}\n')
subprocess.run(['go','build','-o',str(root/'tool.exe'),str(fixture)],check=True,timeout=90)
if public:
 req=urllib.request.Request('https://api.github.com/repos/shindongyoon-siwon/clipcatch/releases/latest',headers={'User-Agent':'ClipCatch-release-QA'})
 with urllib.request.urlopen(req,timeout=20) as r:release=json.load(r)
 assert release['tag_name']=='v'+version and not release['draft'] and not release['prerelease'],release['tag_name']
 asset=next(a for a in release['assets'] if a['name']=='ClipCatch.exe')
 assert asset['digest']=='sha256:'+sha(exe),'Public release is not the tested artifact'
# Independent observation is QA-only. Shipped app contains no COM scan or PowerShell.
# Windows temporary paths may use RUNNER~1 while Explorer exposes runneradmin.
# Compare long paths, then independently require Python samefile on the returned path.
observer=root/'observe.ps1';observer.write_text(r'''
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
Add-Type -TypeDefinition 'using System;using System.Text;using System.Runtime.InteropServices;public class Obs{[DllImport("user32.dll")]public static extern bool IsWindowVisible(IntPtr h);[DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern uint GetLongPathName(string p,StringBuilder b,uint n);public static string Long(string p){var b=new StringBuilder(32768);var n=GetLongPathName(p,b,(uint)b.Capacity);if(n==0||n>=b.Capacity)throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());return b.ToString();}}'
$s=New-Object -ComObject Shell.Application
$expected=[Obs]::Long($env:CC_WATCH_FOLDER)
[Console]::Error.WriteLine('expected='+$expected)
[Console]::WriteLine('ready');[Console]::Out.Flush()
while(!(Test-Path -LiteralPath $env:CC_WATCH_SIGNAL)){Start-Sleep -Milliseconds 20}
$clock=[Diagnostics.Stopwatch]::StartNew();$found=$null;$seen=@{}
while($clock.ElapsedMilliseconds -lt 10000){
 foreach($w in @($s.Windows())){try{
  $p=[string]$w.Document.Folder.Self.Path
  $visible=[Obs]::IsWindowVisible([IntPtr][long]$w.HWND)
  $key=$p+'|'+$visible
  if(!$seen.ContainsKey($key)){$seen[$key]=$true;[Console]::Error.WriteLine('observed='+$key)}
  if($p -and ([Obs]::Long($p) -eq $expected) -and $visible){$found=$w;break}
 }catch{[Console]::Error.WriteLine('observer_error='+$_.Exception.Message)}}
 if($null -ne $found){break};Start-Sleep -Milliseconds 40
}
if($null -eq $found){throw 'Exact visible Explorer folder did not appear'}
[Console]::WriteLine((@{observed=$true;elapsed_ms=$clock.ElapsedMilliseconds;hwnd=[long]$found.HWND;path=[string]$found.Document.Folder.Self.Path}|ConvertTo-Json -Compress));[Console]::Out.Flush()
$command=[Console]::ReadLine()
if($command -eq 'close'){try{$found.Quit()}catch{}}
''',encoding='utf-8')
def folder_click(page,path,label):
 signal=root/(label+'.signal');env=os.environ.copy();env.update(CC_WATCH_FOLDER=path,CC_WATCH_SIGNAL=str(signal))
 errfile=(out/(label+'-observer.log')).open('w',encoding='utf-8')
 obs=subprocess.Popen(['powershell.exe','-NoProfile','-NonInteractive','-File',str(observer)],env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=errfile,text=True,encoding='utf-8')
 try:
  assert read_line(obs)=='ready';signal.touch()
  with page.expect_response(lambda r:r.url==base+'/api/open-folder' and r.request.method=='POST',timeout=8000) as captured:
   page.locator('#folderBtn').click()
  response=captured.value;data=response.json();assert response.ok and data['ok'] and data['requested'] and data['path']==path,data
  assert 'foreground' not in data and 'focus_reason' not in data,'Old focus pipeline returned'
  timing=response.request.timing;api_ms=timing['responseEnd']
  observed=json.loads(read_line(obs,12));assert observed['observed'],observed
  assert Path(observed['path']).samefile(path),'Explorer displayed a different directory'
  expect(page.locator('#folderFeedback')).to_be_hidden();expect(page.locator('#folderBtn')).to_be_enabled()
  assert 0<=api_ms<1500,('Slow direct folder API',api_ms)
  assert observed['elapsed_ms']<5000,('Slow actual Explorer appearance',observed)
  result={'api_ms':round(api_ms,2),'visible_window_ms':observed['elapsed_ms'],'visible_exact_folder':True,'same_directory_identity':True,'chrome_button_clicked':True,'requested_path':path,'observed_path':observed['path']}
  page.screenshot(path=str(out/(label+'-page.png')))
  return result
 finally:
  try:obs.communicate('close\n',timeout=5)
  except subprocess.TimeoutExpired:obs.kill();obs.communicate()
  errfile.close()
log=(out/'userflow.log').open('w',encoding='utf-8');results=[]
cases=[('fresh-ascii',None,'fresh'),('fresh-unicode',None,'fresh')]
if public:cases=[('199-update-button','1.9.9','button'),('199-new-exe','1.9.9','manual'),('198-update-button','1.9.8','button'),('198-new-exe','1.9.8','manual'),('196-update-button','1.9.6','button')]
try:
 with sync_playwright() as pw:
  browser=pw.chromium.launch(channel='chrome',headless=False)
  for label,old_version,mode in cases:
   processes=[];page=browser.new_page();page.on('dialog',lambda d:d.accept())
   try:
    profile=root/label/('User \ud55c\uae00 & (1)' if 'unicode' in label else 'Siwon-D2')
    env=os.environ.copy();env.update(LOCALAPPDATA=str(profile/'local'),APPDATA=str(profile/'roaming'),USERPROFILE=str(profile),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_PORTABLE='0')
    for k in ('LOCALAPPDATA','APPDATA','USERPROFILE'):Path(env[k]).mkdir(parents=True,exist_ok=True)
    (profile/'Desktop').mkdir();data=profile/'local/ClipCatch';tools=data/'tools';tools.mkdir(parents=True)
    for name in ('yt-dlp.exe','ffmpeg.exe','ffprobe.exe'):shutil.copy2(root/'tool.exe',tools/name)
    target=profile/'local/Programs/ClipCatch/ClipCatch.exe';target.parent.mkdir(parents=True)
    saved=profile/'Downloads/ClipCatch/preserve-original.txt';saved.parent.mkdir(parents=True);saved.write_bytes(b'original preserved');saved_hash=sha(saved)
    (data/'history.json').write_text(json.dumps([{'id':'keep-history','status':'done','file':str(saved),'title':'preserve','url':'https://x.com/example/status/123456'}]))
    (data/'output-preference.json').write_text('{"mode":"original"}')
    if old_version:
     shutil.copy2(fetch_old(old_version),target)
     running=subprocess.Popen([str(target)],env=env,stdout=log,stderr=log);processes.append(running)
     wait(lambda:get('/api/health')['version']==old_version)
    else:
     fresh=subprocess.Popen([str(exe)],env=env,stdout=log,stderr=log);processes.append(fresh);assert fresh.wait(timeout=30)==0
     wait(lambda:get('/api/health')['version']==version)
    wait(lambda:get('/api/state')['phase']=='ready')
    page.goto(base,wait_until='domcontentloaded')
    if mode=='button':
     page.wait_for_function("['requiredUpdateInstall','installAppUpdate'].some(id=>{const e=document.getElementById(id);return e&&!e.disabled&&e.getClientRects().length>0;})",timeout=90000)
     button=page.locator('#requiredUpdateInstall' if page.locator('#requiredUpdateDialog').is_visible() else '#installAppUpdate')
     with page.expect_request(lambda r:r.url==base+'/api/install-app-update' and r.method=='POST'):
      button.click()
    elif mode=='manual':
     manual=subprocess.Popen([str(exe)],env=env,stdout=log,stderr=log);processes.append(manual);assert manual.wait(timeout=45)==0
    if old_version:
     wait(lambda:get('/api/health')['version']==version,90)
     wait(lambda:(data/'update-receipt.json').exists() and json.loads((data/'update-receipt.json').read_text()).get('status')=='verified',30)
     assert running.wait(timeout=15)==0
    assert sha(target)==sha(exe),'Installed file differs from final artifact'
    state=get('/api/state');assert state['output_mode']=='original' and any(j['id']=='keep-history'for j in state['jobs']),state
    assert sha(saved)==saved_hash
    page.goto(base,wait_until='domcontentloaded')
    sample=folder_click(page,state['save_dir'],label)
    results.append({'case':label,'from':old_version,'to':version,'update_path':mode,'preservation':True,**sample})
   finally:
    try:page.screenshot(path=str(out/(label+'-final.png')))
    except Exception:pass
    page.close();stop()
    for p in processes:
     if p.poll() is None:p.terminate();p.wait(timeout=10)
    if 'data'in locals() and (data/'updater.log').exists():shutil.copy2(data/'updater.log',out/(label+'-updater.log'))
  browser.close()
finally:
 log.close()
report={'version':version,'published_real_executables':public,'browser':'installed Google Chrome (headed)','no_update_helper_invoked_directly':True,'tool_mode':'startup stand-ins only; actual Explorer and HTTP/update flows','user_pc_tested':False,'results':results}
(out/'userflow-results.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(report,ensure_ascii=False))
