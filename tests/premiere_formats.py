"""Offline selector QA: highest geometry/fps first, AVC/AAC only as tie breakers."""
import json
import pathlib
import subprocess
import sys
import tempfile
SORT='hasvid,res,fps,hdr:12,lang,+vcodec:avc,+acodec:m4a'
def video(name,height,codec,fps=30):
    return dict(format_id=name,url='https://example.invalid/'+name+'.mp4',ext='mp4',width=height*16//9,height=height,fps=fps,vcodec=codec,acodec='none',protocol='https')
def audio(name,codec,bitrate):
    return dict(format_id=name,url='https://example.invalid/'+name+'.m4a',ext='m4a' if codec.startswith('mp4a') else 'webm',vcodec='none',acodec=codec,abr=bitrate,protocol='https')
def selected(formats):
    with tempfile.TemporaryDirectory() as d:
        info=pathlib.Path(d)/'fixture.json'
        info.write_text(json.dumps(dict(id='fixture',title='Offline fixture',extractor='Generic',webpage_url='https://example.invalid/fixture',formats=formats)))
        result=subprocess.run([sys.executable,'-m','yt_dlp','--ignore-config','--load-info-json',str(info),'--simulate','--no-check-formats','--format-sort-force','-S',SORT,'-f','bv*+ba/b','--print','%(format_id)s'],text=True,capture_output=True,timeout=30,check=True)
        return result.stdout.strip().splitlines()[-1]
common=[audio('aac','mp4a.40.2',128),audio('opus','opus',160)]
cases=[('same-resolution-avc',[video('avc1080',1080,'avc1.640028'),video('vp91080',1080,'vp9')]+common,'avc1080+aac'),('preserve-4k',[video('avc1080',1080,'avc1.640028'),video('vp92160',2160,'vp9')]+common,'vp92160+aac'),('preserve-60fps',[video('avc30',1080,'avc1.640028',30),video('vp960',1080,'vp9',60)]+common,'vp960+aac')]
for name,formats,expected in cases:
    actual=selected(formats)
    assert actual==expected,(name,actual,expected)
    print(name+': PASS '+actual)
print('No live site request or media download was performed.')
