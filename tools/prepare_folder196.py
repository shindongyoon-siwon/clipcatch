"""Integrate the approved existing folder-button repair on the release candidate only."""
from pathlib import Path
r=Path(__file__).resolve().parents[1]
assert (r/'VERSION').read_text().strip()=='1.9.6'
p=r/'src/main.go';s=p.read_text(encoding='utf-8')
a=s.index('\tmux.HandleFunc("POST /api/open-folder",');b=s.index('\tmux.HandleFunc("POST /api/stop",',a)
assert 'openFolder(a.saveDir)'in s[a:b]and b-a<900
s=s[:a]+'\tmux.HandleFunc("POST /api/open-folder", a.openSaveFolder)\n\ta.folderRoutes(mux)\n'+s[b:]
a=s.index('func openFolder(path string) error {');b=s.index('func openBrowser(addr string)',a)
assert 'hiddenCommand("explorer.exe", path)'in s[a:b]
s=s[:a]+'func openFolder(path string) error { return openDirectory(path) }\n'+s[b:];p.write_text(s,encoding='utf-8')
p=r/'src/web/index.html';s=p.read_text(encoding='utf-8');old="    $('folderBtn').addEventListener('click', async ()=>{try{await post('/api/open-folder');}catch(e){notice(e.message)}});"
assert s.count(old)==1;s=s.replace(old,'');assert s.count('</body>')==1;s=s.replace('</body>','<script src="/folder-open.js"></script>\n</body>');p.write_text(s,encoding='utf-8')
p=r/'.github/workflows/release.yml';s=p.read_text(encoding='utf-8');s=s.replace("      PYTHONUTF8: '1'","      PYTHONUTF8: '1'\n      CLIPCATCH_FOLDER_NATIVE_TEST: '1'")
old='          python tests/batch_workspace_offline.py\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }';assert s.count(old)==1;s=s.replace(old,old+'\n          python tests/folder_open_ui.py\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }')
old='          Copy-Item release-evidence/candidate-build-info.json dist/build-info.json';assert s.count(old)==1;s=s.replace(old,old+'\n          git archive --format=zip --output=release-evidence/ClipCatch-source.zip HEAD\n          if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }');p.write_text(s,encoding='utf-8')
p=r/'tests/windows_release196.py';s=p.read_text(encoding='utf-8');old=" assert get('/api/quality/status')['engine_ready'];intact(pins2);stop();intact(pins2)";assert s.count(old)==1
s=s.replace(old," assert get('/api/quality/status')['engine_ready'];intact(pins2)\n opened=post('/api/open-folder',{});assert opened['ok']and opened['requested']and opened['path']==get('/api/state')['save_dir'],opened\n stop();intact(pins2)")
s=s.replace("'native_gpu_inference_tested':False","'native_folder_request_accepted':True,'folder_foreground_focus_asserted':False,'native_gpu_inference_tested':False");p.write_text(s,encoding='utf-8')
p=r/'docs/releases/v1.9.6.md';p.write_text(p.read_text(encoding='utf-8')+'\n폴더 열기 오류 수정\n- 탐색기를 숨김 창으로 실행하던 방식을 Windows의 일반 폴더 열기로 교체했습니다.\n- 한글·공백이 포함된 폴더 경로를 그대로 전달하고, 요청 실패나 응답 지연을 화면에 표시합니다. 실패 시 실제 저장 경로를 확인할 수 있습니다.\n- 새 파일 열기 버튼이나 저장 폴더 변경 기능은 추가하지 않았습니다.\n',encoding='utf-8')
print('Existing folder-button fix integrated; production remains unchanged until release checks pass')
