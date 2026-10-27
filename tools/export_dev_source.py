"""Export the exact expanded source used for the candidate; no publishing or git writes."""
import hashlib,json,subprocess,sys,zipfile
from pathlib import Path
R=Path(__file__).resolve().parents[1];out=Path(sys.argv[1]).resolve();out.mkdir(parents=True,exist_ok=True)
files=[]
for base in ['src','tests','tools','docs','dev']:
 for p in (R/base).rglob('*'):
  if p.is_file() and not any(x in p.parts for x in ['quality_bundle','__pycache__']) and p.suffix not in ['.exe','.syso','.pyc','.log']:files.append(p)
for name in ['README.md','VERSION','TEST_VERSION','.gitignore']:
 p=R/name
 if p.exists():files.append(p)
with zipfile.ZipFile(out/'ClipCatch-Dev196-source.zip','w',zipfile.ZIP_DEFLATED)as z:
 for p in files:z.write(p,p.relative_to(R))
hashes={str(p.relative_to(R)):hashlib.sha256(p.read_bytes()).hexdigest()for p in files}
(out/'source-sha256.json').write_text(json.dumps(hashes,indent=2),encoding='utf-8')
try:(out/'source-commit.txt').write_text(subprocess.check_output(['git','rev-parse','HEAD'],cwd=R,text=True),encoding='utf-8')
except (subprocess.SubprocessError,OSError):pass
print('Exported exact candidate source, files:',len(files))
