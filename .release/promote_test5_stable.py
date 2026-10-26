#!/usr/bin/env python3
"""Promote checksum-verified TEST2 test.5 source to stable ClipCatch v1.9.1."""
from pathlib import Path
import json
import sys

root=Path(sys.argv[1] if len(sys.argv)>1 else '.').resolve()
def update(rel, changes):
    path=root/rel
    text=path.read_text(encoding='utf-8')
    for old,new in changes:
        if old not in text:
            raise RuntimeError(f'missing promotion anchor in {rel}: {old[:90]}')
        text=text.replace(old,new)
    path.write_text(text,encoding='utf-8',newline='\n')

(root/'VERSION').write_text('1.9.1\n',encoding='utf-8')
update('src/main.go', [
    ('appDefaultVersion = "1.9.0"', 'appDefaultVersion = "1.9.1"'),
    ('/test-workspace.js','/workspace-tabs.js'),
])
manifest_path=root/'src/browser_extension/manifest.json'
manifest=json.loads(manifest_path.read_text(encoding='utf-8'))
manifest['name']='ClipCatch Browser'
manifest['version']='1.9.1'
manifest['version_name']='1.9.1'
manifest['description']='ClipCatch companion for selecting public videos and downloading them as original files or Premiere-compatible MP4.'
manifest['action']['default_title']='ClipCatch - MP4 / Original'
manifest['action'].pop('default_icon',None)
manifest.pop('key',None)
manifest.pop('icons',None)
manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
for path in (root/'src/browser_extension').glob('icon*.png'):
    path.unlink()

update('src/browser_extension/background.js', [
    ("base.port!=='18766'", "(!base.port||Number(base.port)<1024||Number(base.port)>65535)"),
    ("if(new URL(code.base).port!=='18766')throw Error('정식판 코드는 연결할 수 없습니다. ClipCatch 테스트판에서 코드를 복사해 주세요.');", "if(['18765','18766'].includes(new URL(code.base).port))throw Error('테스트판 연결 주소는 사용할 수 없습니다. 운영판에서 연결 코드를 복사해 주세요.');"),
    ("'clipcatch-test2'", "'clipcatch'"),
    ("channel:'test2'", "channel:'stable'"),
    ("channel:'test'", "channel:'stable'"),
    ("r.channel!=='test'", "r.channel!=='stable'"),
    ("info.channel!=='test'", "info.channel!=='stable'"),
    ("build:'1.9.1.5'", "build:'1.9.1'"),
    ('TEST2', 'ClipCatch'),
    ('정식판이 아닌 ClipCatch를 실행해 주세요.', '운영 중인 ClipCatch를 실행해 주세요.'),
    ('테스트판이 아닌 프로그램입니다.', 'ClipCatch 운영판이 아닌 프로그램입니다.'),
    ('ClipCatch ClipCatch와 먼저 연결해 주세요.', 'ClipCatch와 먼저 연결해 주세요.'),
])
update('src/browser_extension/popup.js', [
    ("hello?.build!=='1.9.1.5'", "hello?.build!=='1.9.1'"),
    ("hello?.channel!=='test2'", "hello?.channel!=='stable'"),
    ('TEST2', 'ClipCatch'),
])
update('src/browser_extension/popup.html', [
    ('<span>TEST2</span>',''),
    ('1.9.1-test.5','1.9.1'),
    ('ClipCatch 테스트판','ClipCatch 프로그램'),
    ('TEST2 연결 확인 중','ClipCatch 연결 확인 중'),
])
(root/'src/browser_extension/README.txt').write_text('ClipCatch Browser v1.9.1\nUse the bundled extension folder in ClipCatch and reload at chrome://extensions after update. Existing production Chrome extension identity remains unchanged. Pair only with stable ClipCatch.\n',encoding='utf-8')
update('src/web/index.html', [
    ('ClipCatch TEST2','ClipCatch'),
    ('ClipCatch Browser TEST2','ClipCatch Browser'),
    ('ClipCatch 테스트판이 종료되었습니다.', 'ClipCatch가 종료되었습니다.'),
    ('ClipCatch-Test2-1.9.1.exe', 'ClipCatch.exe'),
    ('CLIPCATCH TEST WORKSPACE','CLIPCATCH'),
    ('테스트판은 정식판의 설정·쿠키·다운로드 기록과 분리됩니다.','다운로드한 영상과 설정은 내 PC에 보관됩니다.'),
])
update('src/web/browser-link.js', [('ClipCatch Browser TEST2','ClipCatch Browser')])
update('src/web/startup-update.js', [('CLIPCATCH TEST2 UPDATE','CLIPCATCH UPDATE')])
update('src/web/quality.js', [('TEST2 quality workspace','ClipCatch quality workspace')])
update('src/quality_engine.go', [('The distributed TEST2 EXE','The ClipCatch Windows EXE')])

trial_workspace=root/'src/web/test-workspace.js'
workspace=trial_workspace.read_text(encoding='utf-8')
for old,new in [
    ('TEST2 1.9.1-test.5 · 정식판과 분리된 테스트판','ClipCatch v1.9.1 · 영상 다운로드·화질 개선'),
    ('정식판의 설정·인증·기록은 변경하지 않습니다. 저장 폴더: 다운로드 / ClipCatch-Test2.','기존 설정·다운로드 기록은 유지되고, 영상은 다운로드 / ClipCatch에 저장됩니다.'),
    ('테스트판은 자동으로 정식판으로 전환되지 않습니다.','새 운영 버전은 확인 후 설치할 수 있습니다.'),
]:
    if old not in workspace:
        raise RuntimeError('missing workspace anchor: '+old)
    workspace=workspace.replace(old,new)
(root/'src/web/workspace-tabs.js').write_text(workspace,encoding='utf-8',newline='\n')
trial_workspace.unlink()

update('tests/feed-worker.test.mjs', [
    ("simulatedBackend='clipcatch-test2'", "simulatedBackend='clipcatch'"),
    ("channel:'test'", "channel:'stable'"),
    ("'http://127.0.0.1:18766'", "'http://127.0.0.1:8765'"),
    ("simulatedBackend='clipcatch';const r=await ask({kind:'send-selected',keys});", "simulatedBackend='clipcatch-test2';const r=await ask({kind:'send-selected',keys});"),
    ("TEST1 and stable pairing never contact either backend", "TEST2 and TEST1 pairing never contact either backend"),
    ('[8765,18765]', '[18766,18765]'),
    ("test('trial extension refuses production pairing before any request',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:8765#'+'a'.repeat(64)});assert.equal(r.ok,false);assert.equal(calls.length,0);});", "test('stable extension refuses TEST2 pairing before any request',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:18766#'+'a'.repeat(64)});assert.equal(r.ok,false);assert.equal(calls.length,0);});"),
    ("test('trial pairing is explicitly marked and accepted',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:18766#'+'a'.repeat(64)});assert.equal(r.ok,true);assert.equal(calls[0].b.channel,'test');});", "test('stable pairing is explicitly marked and accepted',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:8765#'+'a'.repeat(64)});assert.equal(r.ok,true);assert.equal(calls[0].b.channel,'stable');});"),
    ("assert.equal(r.data.build,'1.9.1.5')", "assert.equal(r.data.build,'1.9.1')"),
    ("assert.equal(r.data.channel,'test2')", "assert.equal(r.data.channel,'stable')"),
])
update('src/browser_multisite_test.go', [('"version": "1.9.1.5"','"version": "1.9.1"')])
for path in [root/'src/web/index.html',root/'src/web/browser-link.js',root/'src/web/startup-update.js',root/'src/web/workspace-tabs.js',*list((root/'src/browser_extension').glob('*'))]:
    if path.suffix.lower() not in ('.js','.json','.html','.txt'):
        continue
    data=path.read_text(encoding='utf-8')
    for bad in ['test2','TEST2','1.9.1-test.5','18766','1.9.1.5']:
        if bad in data and not (path.name=='background.js' and bad=='18766'):
            raise RuntimeError(f'test-only identity {bad} leaked into production bundle: {path}')
trial=(root/'src/trial_runtime.go').read_text(encoding='utf-8')
assert 'var appChannel = "stable"' in trial
assert 'return "ClipCatch-Test2"' in trial
assert 'return "8765"' in trial

readme=root/'README.md'
if readme.exists():
    before=readme.read_text(encoding='utf-8')
    note=('## ClipCatch v1.9.1 (stable)\n\n'
        '- Promoted verified TEST2 1.9.1-test.5 behavior into stable ClipCatch.\n'
        '- New-version update dialog preserves active jobs; new jobs wait for update. Offline check failure does not lock use.\n'
        '- Private temporary workspaces publish only verified MP4 and clean only on success; failed sources and existing user originals remain preserved.\n'
        '- Compatible MP4 files are byte-copied, otherwise lossy transcoding may be needed.\n'
        '- Browser extension identifies build mismatch and offers reload; stable channel pairing and Chrome identity retained.\n'
        '- TEST2 remains an independent prerelease; no server data modified.\n\n')
    readme.write_text('# ClipCatch v1.9.1\n\n'+note+before.replace('# ClipCatch v1.9.0\n','',1),encoding='utf-8')

release_path=root/'.github/workflows/release.yml'
if release_path.exists():
    workflow=release_path.read_text(encoding='utf-8')
    assert '      - name: Test and compile Windows executable' in workflow
    assert '          go test ./...' in workflow
    assert '          CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath' in workflow
    preface=(
        '      - name: Fetch checksum-verified bundled AI runtime\n'
        '        shell: bash\n'
        '        run: |\n'
        '          set -euo pipefail\n'
        '          mkdir -p src/quality_bundle\n'
        "          curl --fail --location --retry 2 --max-time 120 --proto '=https' --proto-redir '=https' https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/realesrgan-ncnn-vulkan-20220424-windows.zip -o src/quality_bundle/windows.zip\n"
        "          echo 'abc02804e17982a3be33675e4d471e91ea374e65b70167abc09e31acb412802d  src/quality_bundle/windows.zip' | sha256sum -c -\n\n"
    )
    workflow=workflow.replace('      - name: Test and compile Windows executable', preface+'      - name: Test and compile Windows executable',1)
    workflow=workflow.replace('          go test ./...', '          go test -tags bundled_ai ./...\n          go vet -tags bundled_ai ./...',1)
    workflow=workflow.replace('          CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath', '          CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags bundled_ai -trimpath',1)
    release_path.write_text(workflow,encoding='utf-8',newline='\n')
print('Promoted verified TEST2 test.5 source to stable ClipCatch v1.9.1.')
