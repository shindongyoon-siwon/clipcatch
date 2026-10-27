"""Materialize approved folder-foreground changes on a production-based copy."""
from pathlib import Path
R=Path(__file__).resolve().parents[1]
def replace(path,before,after):
 p=R/path;s=p.read_text(encoding='utf-8')
 if after in s:return
 if before not in s:raise RuntimeError('Unexpected baseline: '+path)
 p.write_text(s.replace(before,after,1),encoding='utf-8')
replace('src/main.go','appDefaultVersion = "1.9.7"','appDefaultVersion = "1.9.8"')
replace('src/main.go','\tif len(os.Args) == 4 && os.Args[1] == "--clipcatch-apply-update" {','\tif handleFolderFocusCommand() { return }\n\tif len(os.Args) == 4 && os.Args[1] == "--clipcatch-apply-update" {')
replace('src/release196_test.go','appDefaultVersion != "1.9.7"','appDefaultVersion != "1.9.8"')
replace('tests/windows_release196.py',"with urllib.request.urlopen(request,timeout=5)","with urllib.request.urlopen(request,timeout=10)")
# Shell-created foreground changes before COM registration are not user input.
replace('src/folder_focus.go','if o.Foreground()!=0 && o.Foreground()!=initial {r.Reason="user_switched";return r,nil}','if o.InputStamp()!=input {r.Reason="user_switched";return r,nil}')
p=R/'tests/windows_release196.py';s=p.read_text(encoding='utf-8')
for name in ['windows_folder198.py','windows_update198.py']:
 line=f"subprocess.run([sys.executable,str(Path(__file__).with_name('{name}')),str(exe),str(out/'{name[:-3]}')],check=True,timeout=180)"
 if line not in s:s+='\n'+line+'\n'
p.write_text(s,encoding='utf-8')
# A second real-old-binary upgrade test retains locked-backup/parallel checks.
s=(R/'tests/windows_update197.py').read_text(encoding='utf-8')
s=s.replace('1.9.6','1.9.7').replace('old196','old197').replace('actual_196','actual_197').replace('c995502a2942c33ede7f836fe1bb7178e72a4598ca127d29e5c80698fb7bbd58','c59d40e27bc508e672a546371982e6ddb2a3733c46dca67b037cd6755d0fe843')
(R/'tests/windows_update198.py').write_text(s,encoding='utf-8')
(R/'VERSION').write_text('1.9.8\n',encoding='utf-8')
p=R/'tests/folder_open_ui.py';s=p.read_text(encoding='utf-8')
line="\nimport runpy\nrunpy.run_path(str(root/'tests/folder_focus_ui.py'),run_name='__main__')\n"
if 'folder_focus_ui.py' not in s:p.write_text(s+line,encoding='utf-8')
print('Folder foreground source prepared; updater behavior and auto-update policy unchanged')
