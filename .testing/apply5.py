from pathlib import Path
import json
root=Path(__file__).resolve().parents[1]
def change(name,old,new):
 p=root/name;s=p.read_text(encoding='utf-8');assert old in s,(name,old[:70]);p.write_text(s.replace(old,new,1),encoding='utf-8',newline='\n')
change('src/main.go','type Job struct {','type Job struct {\n\tDownloadDir string `json:"download_dir,omitempty"`')
change('src/main.go','"-P", a.saveDir,','"-P", a.downloadRoot(j),')
change('src/main.go','\tif j.BrowserSource {\n\t\ta.runBrowserJob(id, j)','\tif err := a.prepareManagedDownload(j); err != nil {\n\t\ta.mutateJob(id, func(job *Job) { job.Status="error"; job.ErrorCode="CC-402"; job.Detail="MP4 temporary workspace unavailable" })\n\t\treturn\n\t}\n\tdefer a.finishManagedDownload(id)\n\tif j.BrowserSource {\n\t\ta.runBrowserJob(id, j)')
change('src/premiere.go','result, e := makeEditCopy(ctx, ffmpeg, source,','result, e := a.makeDownloadEditCopy(j, ctx, ffmpeg, source,')
change('src/post_media.go','!validPostSource(a.saveDir, item.Original)','!validPostSource(a.downloadRoot(j), item.Original)')
change('src/post_media.go','result, err := makeEditCopy(ctx, ff, item.Original,','result, err := a.makeDownloadEditCopy(j, ctx, ff, item.Original,')
change('src/premiere.go','\t\tj.File = result.File\n\t\tj.EditReady = true','\t\tj.File = result.File\n\t\tif len(j.Media) == 1 {\n\t\t\tj.Media[0].File=result.File; j.Media[0].EditReady=true\n\t\t}\n\t\tj.EditReady = true')
change('src/premiere.go','\t\t\tid, err := a.queueEditFile(item.Original, j.URL, in.Safe)','\t\t\tsource := item.Original\n\t\t\tif !nonempty(source) { source=item.File }\n\t\t\tid, err := a.queueEditFile(source, j.URL, in.Safe)')
p=root/'src/browser_extension/manifest.json';m=json.loads(p.read_text(encoding='utf-8'));m['version']='1.9.1.5';m['version_name']='1.9.1-test.5';p.write_text(json.dumps(m,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
for p in (root/'src').rglob('*'):
 if p.is_file() and p.suffix in ('.go','.js','.html','.txt'):
  s=p.read_text(encoding='utf-8');t=s.replace('1.9.1-test.4','1.9.1-test.5').replace('1.9.1.4','1.9.1.5')
  if t!=s:p.write_text(t,encoding='utf-8',newline='\n')
for name in ['managed_download.go','update_gate.go','revision5_test.go','web/startup-update.js']:
 p=root/'src'/name;assert not p.exists();p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes((root/'.testing/v5'/name).read_bytes())
change('src/main.go','type ClientState struct {','type ClientState struct {\n\tUpdateRequired bool `json:"update_required"`')
change('src/main.go','\t\tstate := a.state','\t\tstate := a.state\n\t\tstate.UpdateRequired = a.updateRequiredLocked()')
change('src/main.go','\ta.mux.ServeHTTP(w, r)','\tif a.updateGateRequest(w,r) { return }\n\ta.mux.ServeHTTP(w, r)')
change('src/main.go','\tif updateInProgress(a.state.AppUpdate.Status) {','\tif a.newWorkBlockedLocked() {')
change('src/browser_bridge.go','\tif r.URL.Path == "/bridge/status" {','\tif a.updateGateRequest(w,r) { return true }\n\tif r.URL.Path == "/bridge/status" {')
change('src/main.go','\ta.qualityRoutes(mux)','\ta.qualityRoutes(mux)\n\tmux.HandleFunc("GET /startup-update.js", func(w http.ResponseWriter,r *http.Request){ b,e:=pages.ReadFile("web/startup-update.js");if e!=nil{http.NotFound(w,r);return};w.Header().Set("Content-Type","text/javascript; charset=utf-8");w.Write(b) })')
change('src/main.go','<script src=\\"/test-workspace.js\\"></script></body>','<script src=\\"/test-workspace.js\\"></script><script src=\\"/startup-update.js\\"></script></body>')
change('src/updater.go','a.state.AppUpdate.Status != "available" || a.latestRelease.BinaryURL == ""','(a.state.AppUpdate.Status != "available" && a.state.AppUpdate.Status != "error") || !a.updateRequiredLocked()')
change('src/trial_update_test.go','rel.Version = "1.9.1-test.5"','rel.Version = "1.9.1-test.6"')
r=root
p=r/'src/browser_extension/background.js';s=p.read_text(encoding='utf-8');s=s.replace('async function handle(m){','async function handle(m){\n if(m.kind===\'hello\')return {build:\'1.9.1.5\',channel:\'test2\',capabilities:[\'output-mode\',\'job-status\']};',1);p.write_text(s,encoding='utf-8',newline='\n')
p=r/'src/browser_extension/popup.js';s=p.read_text(encoding='utf-8');s=s.replace('let latestJobs=[]','let protocolReady=false;\nlet latestJobs=[]',1)
s=s.replace("const e=Error(r?.error||'Request failed');", "const raw=r?.error||'Request failed';const mismatch=raw==='Unknown request'||String(raw).includes('CC-EXT-VERSION');if(mismatch)$('reloadExtension').hidden=false;const e=Error(mismatch?'확장프로그램 화면과 실행부 버전이 다릅니다. [확장 새로고침]을 누른 뒤 다시 열어 주세요.':raw);",1)
s=s.replace("working=true;buttons();\n try{\n  const preference", "working=true;buttons();\n try{\n  const hello=await message({kind:'hello'});\n  if(hello?.build!=='1.9.1.5'||hello?.channel!=='test2'){ $('reloadExtension').hidden=false;throw Error('CC-EXT-VERSION: 확장 새로고침 후 다시 열어 주세요.');}\n  protocolReady=true;$('reloadExtension').hidden=true;\n  const preference",1)
s=s.replace('if(polling||working)return;', 'if(polling||working||!protocolReady)return;',1)
s=s.replace("$('openDownloads').onclick=openDownloads;", "$('openDownloads').onclick=openDownloads;\n$('reloadExtension').onclick=()=>chrome.runtime.reload();",1)
s=s.replace('async function load(){\n working=true;buttons();','async function load(){\n protocolReady=false;working=true;buttons();',1)
p.write_text(s,encoding='utf-8',newline='\n')
p=r/'src/browser_extension/popup.html';s=p.read_text(encoding='utf-8');s=s.replace('<p id="connectionBadge"','<button type="button" id="reloadExtension" hidden>확장 새로고침</button><p id="connectionBadge"',1)
s=s.replace('원본은 보존됩니다.', '새 MP4 다운로드는 검증 후 MP4만 남기고 중간 파일을 정리합니다. 직접 선택한 기존 파일은 삭제하지 않습니다.')
p.write_text(s,encoding='utf-8',newline='\n')
p=r/'src/browser_status.go';s=p.read_text(encoding='utf-8').replace('"phase": a.state.Phase,','"phase": a.state.Phase, "update_required": a.updateRequiredLocked(),');p.write_text(s,encoding='utf-8',newline='\n')
p=r/'tests/feed-worker.test.mjs';s=p.read_text(encoding='utf-8')+"\ntest('worker exposes exact build before popup sends new commands',async()=>{await setup();const r=await ask({kind:'hello'});assert.equal(r.ok,true);assert.equal(r.data.build,'1.9.1.5');assert.equal(r.data.channel,'test2');assert.equal(calls.length,0);});\n";p.write_text(s,encoding='utf-8',newline='\n')
change('src/premiere.go','filepath.Base(item.Original)','filepath.Base(source)')
change('src/browser_bridge.go','updateInProgress(a.state.AppUpdate.Status)','a.newWorkBlockedLocked()')
for name in ['premiere.go','quality.go']:
 p=root/'src'/name;v=p.read_text(encoding='utf-8')
 v=v.replace('a.state.Phase != "ready" || a.state.AppUpdate.Status == "downloading" || a.state.AppUpdate.Status == "installing"','a.state.Phase != "ready" || a.newWorkBlockedLocked()').replace('a.state.Phase != "ready" || a.state.AppUpdate.Status == "installing" || a.state.AppUpdate.Status == "downloading"','a.state.Phase != "ready" || a.newWorkBlockedLocked()')
 p.write_text(v,encoding='utf-8',newline='\n')
p=root/'src/post_media_test.go';s=p.read_text(encoding='utf-8')
old='script := fmt.Sprintf("#!/bin/sh\\nprintf \'CLIP_FILE|%s\\\\n\'\\nprintf \'CLIP_FILE|%s\\\\n\'\\n", p, two)';assert old in s
shell='#!/bin/sh\nwhile [ "$#" -gt 0 ]; do if [ "$1" = "-P" ]; then shift; dest=$1; fi; shift; done\ncp %q "$dest/a.mkv"\ncp %q "$dest/b.mkv"\nprintf \'CLIP_FILE|%%s\\n\' "$dest/a.mkv" "$dest/b.mkv"\n'
s=s.replace(old,'script := fmt.Sprintf('+json.dumps(shell)+', p, two)');p.write_text(s,encoding='utf-8',newline='\n')
print('Prepared TEST2 test.5 only. Stable source and releases are unchanged.')
