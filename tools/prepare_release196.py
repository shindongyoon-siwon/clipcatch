"""Materialize approved, pinned DEV196 changes into a fresh v1.9.5 release copy.
Never publishes, modifies a Git ref, or copies development identity/installer files.
"""
import argparse, json, subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--source',required=True);p.add_argument('--target',required=True);a=p.parse_args()
s=Path(a.source).resolve();r=Path(a.target).resolve()
assert s!=r
assert (r/'VERSION').read_text().strip()=='1.9.5','Unexpected production baseline'
files='''src/main.go
src/lifecycle.go
src/web/lifecycle.js
src/web/index.html
src/web/quality.js
src/quality.go
src/quality_media.go
src/quality_batch.go
src/quality_batch_test.go
src/quality_upload.go
src/quality_preview_save.go
src/quality_retention.go
src/quality_retention_test.go
src/queue_controls.go
src/workspace_quality_test.go
src/update_gate.go
tests/batch_workspace_offline.py
tools/windows_resources.py'''.splitlines()
for name in files:
 dest=r/name;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text((s/name).read_text(encoding='utf-8'),encoding='utf-8')
def replace(path,before,after):
 f=r/path;t=f.read_text(encoding='utf-8');assert t.count(before)==1,(path,before[:80],t.count(before));f.write_text(t.replace(before,after),encoding='utf-8')
replace('src/main.go','\tBuildLabel string `json:"build_label"`\n\tIsolated   bool   `json:"isolated_development"`\n\n','')
replace('src/main.go','\t\tBuildLabel:      buildLabel(),\n\t\tIsolated:        isolatedDevelopment,\n','')
replace('src/main.go','\tif isolatedDevelopment {\n\t\ta.state.AppUpdate = UpdateInfo{Status: "current", Detail: "DEV196: no automatic updates"}\n\t\ta.state.DistributionURL = ""\n\t}\n','')
replace('src/main.go','\tif isolatedDevelopment && os.Getenv("CLIPCATCH_DEV196_FIXTURE") == "1" {\n\t\ta.ytDlp = os.Getenv("CLIPCATCH_DEV196_YTDLP")\n\t\ta.ffmpeg = os.Getenv("CLIPCATCH_DEV196_FFMPEG")\n\t\ta.setSetup("ready", "DEV196 fixture tools", "", 100)\n\t\treturn\n\t}\n','')
replace('src/main.go','\tif isolatedDevelopment && len(os.Args) > 1 {\n\t\tlog.Println("Development installer commands refused")\n\t\treturn\n\t}\n','')
replace('src/main.go','(isTrialBuild() || isolatedDevelopment)','isTrialBuild()')
replace('src/main.go','a.requireStartupCheck = !isolatedDevelopment','a.requireStartupCheck = true')
replace('src/main.go','appDefaultVersion = "1.9.2"','appDefaultVersion = "1.9.6"')
replace('src/web/lifecycle.js'," const before=render;render=function(st){before(st);if(st?.isolated_development){document.title='ClipCatch DEV196';for(const id of ['shareStable','checkAppUpdate','navExtension'])document.getElementById(id)?.setAttribute('hidden','');}};if(lastState)render(lastState);\n",'')
q=r/'tests/batch_workspace_offline.py';t=q.read_text(encoding='utf-8');t=t.replace('1.9.6-test.1','1.9.6').replace("channel:'test',build_label:'DEV196',isolated_development:true","channel:'stable',build_label:'',isolated_development:false").replace('ClipCatch-Dev196','ClipCatch');q.write_text(t,encoding='utf-8')
m=r/'src/browser_extension/manifest.json';v=json.loads(m.read_text());v['version']='1.9.6';v['version_name']='1.9.6';m.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
(r/'VERSION').write_text('1.9.6\n')
t=r/'src/browser_multisite_test.go';v=t.read_text(encoding='utf-8');assert v.count('"version": "1.9.5"')==2;v=v.replace('"version": "1.9.5"','"version": "1.9.6"');t.write_text(v,encoding='utf-8')
g=r/'.gitignore';g.write_text(g.read_text()+'\n# Generated native resources and release evidence\nsrc/rsrc*.syso\nsrc/winres/\nsrc/quality_bundle/\nrelease-evidence/\n.release-dev/\nqa-output/\n',encoding='utf-8')
for name in ['src/trial_runtime.go','src/build_identity.go','src/updater.go','src/update_policy.go','src/browser_bridge.go']:
 assert 'isolatedDevelopment' not in (r/name).read_text(),name
assert not list((r/'src').glob('development_*.go'))
for f in (r/'src').rglob('*'):
 if f.is_file() and not f.name.endswith('_test.go') and f.suffix in ['.go','.js','.html']:
  assert 'isolatedDevelopment' not in f.read_text(encoding='utf-8'),f
subprocess.run(['gofmt','-w',*[str(f)for f in (r/'src').glob('*.go')]],check=True)
print(json.dumps({'materialized':files,'version':'1.9.6','production_identity_from':'v1.9.5','development_identity_copied':False}))
