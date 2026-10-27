"""Real executable/browser lifecycle in a disposable profile; no mocked app API.
Tool stand-ins isolate startup, not download/media correctness. Windows runs
against the exact stable EXE artifact; Linux is an additional source-build check.
"""
import hashlib,json,os,shutil,subprocess,sys,tempfile,time,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect,Error
from PIL import Image
exe,out=map(lambda p:Path(p).resolve(),sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
repo=Path(__file__).resolve().parents[1];version=(repo/'VERSION').read_text().strip()
root=Path(tempfile.mkdtemp(prefix='clipcatch-lifecycle200-'));base='http://127.0.0.1:8765'
env=os.environ.copy();env.update(LOCALAPPDATA=str(root/'local'),APPDATA=str(root/'roaming'),USERPROFILE=str(root/'profile'),HOME=str(root/'home'),XDG_CONFIG_HOME=str(root/'config'),CLIPCATCH_NO_BROWSER='1',CLIPCATCH_PORTABLE='1')
for k in ('LOCALAPPDATA','APPDATA','USERPROFILE','HOME','XDG_CONFIG_HOME'):Path(env[k]).mkdir(parents=True)
data=Path(env['LOCALAPPDATA'] if os.name=='nt' else env['XDG_CONFIG_HOME'])/'ClipCatch';tools=data/'tools';tools.mkdir(parents=True)
suffix='.exe' if os.name=='nt' else ''
fixture=root/'fixture.go';fixture.write_text('package main\nimport "fmt"\nfunc main(){fmt.Println("2026.10.27")}\n')
subprocess.run(['go','build','-o',str(root/('tool'+suffix)),str(fixture)],check=True,timeout=90)
for n in ('yt-dlp','ffmpeg','ffprobe'):shutil.copy2(root/('tool'+suffix),tools/(n+suffix))
preserved=data/'keep-original.txt';preserved.write_text('preserve me')
log=(out/'lifecycle200.log').open('w',encoding='utf-8');processes=[];results={}
def get(path):
 with urllib.request.urlopen(base+path,timeout=3) as r:return json.load(r)
def wait(fn,timeout=45):
 deadline=time.monotonic()+timeout;last=None
 while time.monotonic()<deadline:
  try:
   value=fn()
   if value:return value
  except (OSError,ValueError) as e:last=e
  time.sleep(.1)
 raise AssertionError(f'Condition timed out: {last}')
def start():
 proc=subprocess.Popen([str(exe)],env=env,stdout=log,stderr=log);processes.append(proc)
 wait(lambda:get('/api/health')['version']==version)
 wait(lambda:get('/api/state')['phase']=='ready' and not get('/api/state')['update_checking'],60)
 return proc
def tabs(n):return get('/api/workspace/status').get('clients')==n
try:
 with sync_playwright() as pw:
  opts={'channel':'chrome','headless':False} if os.name=='nt' else {'executable_path':shutil.which('chromium'),'headless':True}
  browser=pw.chromium.launch(**opts)
  context=browser.new_context();page=context.new_page();errors=[]
  page.on('pageerror',lambda e:errors.append(str(e)))
  proc=start();page.goto(base,wait_until='domcontentloaded');wait(lambda:tabs(1))
  cache=data/'activity-update.json';wait(cache.exists)
  first_cache=cache.read_bytes()
  # No duplicate process or duplicate external check on icon launch/resume.
  subprocess.run([str(exe)],env=env,stdout=log,stderr=log,check=True,timeout=15)
  assert proc.poll() is None
  checks=[];page.on('request',lambda r:checks.append(r.url) if r.url.endswith('/api/check-app-update') else None)
  page.locator('#navQuality').click();page.locator('#navDownload').click()
  page.reload(wait_until='domcontentloaded');wait(lambda:tabs(1));page.wait_for_timeout(16000)
  assert proc.poll() is None and get('/api/health')['version']==version
  assert cache.read_bytes()==first_cache and not checks,checks
  results.update(reload_survives_grace=True,duplicate_launch_reuses_running_app=True,activity_within_hour_no_check=True)
  # A second actual tab remains alive when the first is closed.
  second=context.new_page();second.goto(base,wait_until='domcontentloaded');wait(lambda:tabs(2))
  page.close();wait(lambda:tabs(1));second.wait_for_timeout(16000)
  assert proc.poll() is None
  blank=context.new_page();blank.goto('about:blank');blank.bring_to_front();blank.wait_for_timeout(16000)
  assert proc.poll() is None and get('/api/health')['version']==version
  results.update(other_tab_prevents_exit=True,background_tab_prevents_exit=True)
  began=time.monotonic();second.close();proc.wait(timeout=25)
  elapsed=time.monotonic()-began
  assert proc.returncode==0 and 14<=elapsed<=24,elapsed
  assert preserved.read_text()=='preserve me'
  results.update(last_tab_actual_process_exits=True,last_tab_seconds=round(elapsed,2))
  # Reopening uses the persisted one-hour check, and dirty files warn on reload.
  proc=start();page=context.new_page();page.goto(base,wait_until='domcontentloaded');wait(lambda:tabs(1))
  assert cache.read_bytes()==first_cache,'Restart performed a duplicate check'
  results['restart_reuses_recent_update_check']=True
  image=root/'unsaved.png';Image.new('RGB',(8,8),(60,80,100)).save(image)
  page.locator('#navQuality').click();page.locator('#qualityFiles').set_input_files(str(image))
  expect(page.locator('#qualityList .q-item')).to_have_count(1)
  page.wait_for_function('window.clipcatchHasUnsavedQuality()===true')
  dialogs=[]
  def dismiss(d):dialogs.append(d.type);d.dismiss()
  page.on('dialog',dismiss)
  try:page.reload(wait_until='domcontentloaded',timeout=4000)
  except Error as e:
   if 'ERR_ABORTED' not in str(e) and 'Timeout' not in str(e):raise
  assert 'beforeunload' in dialogs,dialogs
  expect(page.locator('#qualityList .q-item')).to_have_count(1)
  page.remove_listener('dialog',dismiss)
  results['unsaved_input_navigation_can_be_cancelled']=True
  page.close();proc.wait(timeout=25)
  assert not errors,errors
  context.close();browser.close()
finally:
 for proc in processes:
  if proc.poll() is None:proc.terminate();proc.wait(timeout=15)
 log.close()
report={'version':version,'platform':sys.platform,'artifact_sha256':hashlib.sha256(exe.read_bytes()).hexdigest(),'real_user_pc_tested':False,'media_tools':'startup stand-ins only','results':results}
(out/'lifecycle200.json').write_text(json.dumps(report,indent=2),encoding='utf-8');print(json.dumps(report))
