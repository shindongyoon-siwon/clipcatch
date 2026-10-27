"""Apply narrowly approved source edits in the independent candidate branch."""
from pathlib import Path
p=Path('.')
for f in p.glob('src/folder_focus*.go'): f.unlink()
for name in ('tests/folder_focus_ui.py','tests/windows_folder198.py'):
 (p/name).unlink()
f=p/'src/main.go';s=f.read_text(encoding='utf-8');assert 'appDefaultVersion = "1.9.8"'in s
s=s.replace('appDefaultVersion = "1.9.8"','appDefaultVersion = "1.9.9"');s=s.replace('\tif handleFolderFocusCommand() {\n\t\treturn\n\t}\n','');f.write_text(s,encoding='utf-8')
(p/'VERSION').write_text('1.9.9\n')
f=p/'src/release196_test.go';f.write_text(f.read_text().replace('appDefaultVersion != "1.9.8"','appDefaultVersion != "1.9.9"'))
f=p/'src/folder_open.go';s=f.read_text(encoding='utf-8').replace('\t"context"\n','')
s=s.replace('serveFocusedSaveFolder(w, r, a.saveDir, openDirectoryFocusedNative)','serveOpenSaveFolder(w, r, a.saveDir, openDirectoryNative)')
a=s.index('// The old error-only adapter');b=s.index('\tfail :=',a)
s=s[:a]+'// Request Explorer directly; do not wait for window discovery or activation.\nfunc serveOpenSaveFolder(w http.ResponseWriter, r *http.Request, path string, launch func(string) error) {\n'+s[b:]
s=s.replace('result, err := launch(r.Context(), full)\n\tif err != nil {','err = launch(full)\n\tif err != nil {')
a=s.index('\tif !result.Requested');b=s.index('\n}',a)
s=s[:a]+'\t// Start acceptance is not proof that a visible window has appeared.\n\twriteJSON(w, 200, map[string]any{"ok": true, "requested": true, "path": full})'+s[b:];f.write_text(s,encoding='utf-8')
f=p/'src/web/folder-open.js';s=f.read_text(encoding='utf-8');a=s.index('   if(data.foreground');b=s.index('\n  }catch(error)',a)
s=s[:a]+"   // Accepted does not mean observed: do not claim the window is visible.\n   feedback.textContent='';feedback.hidden=true;"+s[b:];f.write_text(s,encoding='utf-8')
f=p/'tests/folder_open_ui.py';s=f.read_text(encoding='utf-8');s=s[:s.index('\nimport runpy')];s=s.replace("expect(page.locator('#folderFeedback')).not_to_contain_text('shell failed')","expect(page.locator('#folderFeedback')).to_be_hidden()");f.write_text(s,encoding='utf-8')
f=p/'tests/windows_release196.py';s=f.read_text(encoding='utf-8');s=s.replace("subprocess.run([sys.executable,str(Path(__file__).with_name('windows_folder198.py')),str(exe),str(out/'windows_folder198')],check=True,timeout=180)","subprocess.run([sys.executable,str(Path(__file__).with_name('windows_userflow199.py')),str(exe),str(out/'browser-folder199')],check=True,timeout=180)")
s+="\nsubprocess.run([sys.executable,str(Path(__file__).with_name('windows_update198.py')),str(exe),str(out/'upgrade-from198'),'1.9.8'],check=True,timeout=180)\n";f.write_text(s,encoding='utf-8')
f=p/'tests/windows_update198.py';s=f.read_text(encoding='utf-8');s=s.replace("old=root/'old197.exe';url='https://github.com/shindongyoon-siwon/clipcatch/releases/download/v1.9.7/ClipCatch.exe'\nexpected='c59d40e27bc508e672a546371982e6ddb2a3733c46dca67b037cd6755d0fe843'", "old_version=sys.argv[3] if len(sys.argv)>3 else '1.9.7'\nhashes={'1.9.7':'c59d40e27bc508e672a546371982e6ddb2a3733c46dca67b037cd6755d0fe843','1.9.8':'8477eaa0350d6efd0531a3d0e9dd9b524aa9f5cefd0c0e044927de997e571f40'}\nold=root/'old.exe';url=f'https://github.com/shindongyoon-siwon/clipcatch/releases/download/v{old_version}/ClipCatch.exe'\nexpected=hashes[old_version]")
s=s.replace("get('/api/health')['version']=='1.9.7'", "get('/api/health')['version']==old_version").replace("'actual_197_was_running':True", "'actual_old_version':old_version,'actual_old_was_running':True");f.write_text(s,encoding='utf-8')
f=p/'src/folder_open_test.go';s=f.read_text(encoding='utf-8');s+='''
func TestFolderNoHelperOrForegroundLoop(t *testing.T) {
 main, _ := os.ReadFile("main.go")
 route, _ := os.ReadFile("folder_open.go")
 js, _ := pages.ReadFile("web/folder-open.js")
 for _, token := range []string{"handleFolderFocusCommand", "openDirectoryFocusedNative", "focus_reason", "data.foreground"} {
  if strings.Contains(string(main)+string(route)+string(js), token) { t.Fatal("old focus path remains", token) }
 }
 for _,name:=range []string{"folder_focus.go","folder_focus_windows.go"} {
  if _,err:=os.Stat(name);!os.IsNotExist(err){t.Fatal("unnecessary helper remains",name)}
 }
}
''';f.write_text(s,encoding='utf-8')
print('Direct folder launch prepared without changing updater transaction or production branch')
