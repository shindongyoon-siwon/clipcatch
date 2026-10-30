"""Use the approved SVG geometry unchanged for native Windows resources."""
import argparse,json,subprocess
from pathlib import Path
from PIL import Image,ImageDraw
R=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--version',required=True);p.add_argument('--development',action='store_true');p.add_argument('--test2',action='store_true');p.add_argument('--prepare-only',action='store_true');a=p.parse_args()
if a.development and a.test2:raise SystemExit('Choose one development identity')
svg=(R/'src/web/favicon.svg').read_text()
for token in ['#D9FF57','M32 14V40M21 29L32 40L43 29','M16 44V50H48V44']:
 if token not in svg:raise SystemExit('Icon source changed; review it first')
o=R/'src/winres';o.mkdir(exist_ok=True);s=16;im=Image.new('RGBA',(64*s,64*s),(0,0,0,0));d=ImageDraw.Draw(im);d.rounded_rectangle((s,s,63*s,63*s),radius=18*s,fill='#D9FF57')
def stroke(points):
 pts=[(x*s,y*s)for x,y in points];d.line(pts,fill='#11171E',width=int(5.5*s),joint='curve');r=2.75*s
 for x,y in pts:d.ellipse((int(x-r),int(y-r),int(x+r),int(y+r)),fill='#11171E')
stroke([(32,14),(32,40)]);stroke([(21,29),(32,40),(43,29)]);stroke([(16,44),(16,50),(48,50),(48,44)])
im=im.resize((256,256),Image.Resampling.LANCZOS);im.save(o/'icon.png');im.save(o/'icon.ico',sizes=[(16,16),(24,24),(32,32),(48,48),(64,64),(128,128),(256,256)])
fixed=a.version.split('-')[0]+'.'+(a.version.rsplit('.',1)[1]if '-'in a.version else '0');name='ClipCatch TEST2'if a.test2 else ('ClipCatch DEV196'if a.development else 'ClipCatch');exe='ClipCatch-Test2.exe'if a.test2 else ('ClipCatch-Dev196.exe'if a.development else 'ClipCatch.exe')
info={'ProductName':('ClipCatch TEST2'if a.test2 else 'ClipCatch'),'FileDescription':name,'FileVersion':a.version,'ProductVersion':a.version,'OriginalFilename':exe,'InternalName':name}
res={'RT_GROUP_ICON':{'APP':{'0000':'icon.ico'}},'RT_VERSION':{'#1':{'0000':{'fixed':{'file_version':fixed,'product_version':fixed,'flags':'Prerelease'if (a.development or a.test2) else ''},'info':{'0409':info,'0412':info}}}}}
(o/'winres.json').write_text(json.dumps(res,indent=2)+'\n',encoding='utf-8')
if not a.prepare_only:subprocess.run(['go','run','github.com/tc-hib/go-winres@v0.3.3','make','--arch','amd64','--in','winres/winres.json','--out','rsrc'],cwd=R/'src',check=True)
print(json.dumps({'icon':'approved lime geometry','description':name,'version':a.version}))
