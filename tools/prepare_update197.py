"""Materialize reviewable v1.9.6-based updater repair on the candidate branch only."""
from pathlib import Path
R=Path(__file__).resolve().parents[1]
def edit(name,before,after):
 p=R/name;s=p.read_text(encoding='utf-8')
 if after in s:return
 if before not in s:raise RuntimeError('Unexpected baseline: '+name+' / '+before[:70])
 p.write_text(s.replace(before,after,1),encoding='utf-8')
p=R/'src/updater.go';s=p.read_text(encoding='utf-8')
if '// The updater runs from' in s:
 start=s.index('// The updater runs from');end=s.index('\nfunc logUpdate(',start)
 s=s[:start]+'''// The legacy two-argument command remains compatible with 1.9.5/1.9.6.
func applyClipCatchUpdate(args []string) {
 if runtime.GOOS!="windows"||len(args)!=2{return}
 target:=installedExePath()
 if !sameUpdatePath(args[0],target)||compareRuntimeVersions(args[1],appVersion)!=0{return}
 self,e:=os.Executable();if e!=nil||sameUpdatePath(self,target){return}
 if e=performSafeUpdate(self,target);e!=nil{reportSafeUpdateFailure(e)}
}
'''+s[end:];p.write_text(s,encoding='utf-8')
edit('src/updater.go','if err = stopExistingInstanceForUpgrade(); err != nil {\n\t\treturn err\n\t}','// The new helper owns the cross-process lock and discovers exact-path\n\t// instances (including legacy alternate ports) before requesting shutdown.')
edit('src/updater.go','newFile := filepath.Join(a.dataDir, "updates", "new-"+runtimeUpdateAssetName())','candidateDir, err := os.MkdirTemp(a.dataDir, "update-candidate-")\n\tif err != nil { a.setUpdate(UpdateInfo{Status:"error",Detail:err.Error()}); return }\n\tnewFile := filepath.Join(candidateDir, runtimeUpdateAssetName())')
edit('src/updater.go','cmd := hiddenCommand(newFile, "--clipcatch-apply-update", target, rel.Version)','if err = verifyInstallerIdentity(newFile, rel.Version); err != nil {\n\t\ta.setUpdate(UpdateInfo{Status:"error",Detail:"Installer identity check failed: "+err.Error()}); return\n\t}\n\tcmd := hiddenCommand(newFile, "--clipcatch-apply-update", target, rel.Version)')
edit('src/main.go','\tif installAndRelaunch() {','\tif runtime.GOOS=="windows" && updateLaunchGate() { return }\n\tif installAndRelaunch() {')
edit('src/main.go','\tif err != nil {\n\t\tlistener, err = net.Listen("tcp", "127.0.0.1:0")\n\t}','\t// Do not create a hidden second instance on an untracked random port.\n\tif err != nil {\n\t\tshowInstallError("ClipCatch address is occupied. No second instance or update was started.")\n\t\treturn\n\t}')
edit('src/main.go','appDefaultVersion = "1.9.6"','appDefaultVersion = "1.9.7"')
edit('VERSION','1.9.6','1.9.7')
edit('src/release196_test.go','appDefaultVersion != "1.9.6"','appDefaultVersion != "1.9.7"')
edit('tests/windows_release196.py'," stop();oldproc.wait(timeout=15)\n identity(exe,env,'candidate')"," # Keep the actual old program RUNNING; the new helper must stop and wait.\n identity(exe,env,'candidate')")
edit('tests/windows_release196.py'," wait(lambda:get('/api/health').get('version')==version)\n receipt="," oldproc.wait(timeout=15)\n wait(lambda:get('/api/health').get('version')==version)\n receipt=")
p=R/'tests/windows_release196.py';s=p.read_text(encoding='utf-8')
if "with_name('windows_update197.py')" not in s:
 s+="\nsubprocess.run([sys.executable,str(Path(__file__).with_name('windows_update197.py')),str(exe),str(out/'repair197')],check=True,timeout=180)\n";p.write_text(s,encoding='utf-8')
print('Materialized explicit updater-only changes; production and release refs are untouched')
