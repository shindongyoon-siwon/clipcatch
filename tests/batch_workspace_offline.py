"""DOM/event fixtures; does not claim to test the OS clipboard or real inference."""
import json,os,shutil
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
R=Path(__file__).resolve().parents[1];W=R/'src/web';out=Path(os.getenv('CLIPCATCH_QA_OUTPUT',str(R/'qa-output')));out.mkdir(parents=True,exist_ok=True)
mock=r'''
window.__qa={calls:[],sources:[],state:{version:'1.9.6',channel:'stable',build_label:'',isolated_development:false,phase:'ready',jobs:[],save_dir:'C:\\Downloads\\ClipCatch',output_mode:'premiere',app_update:{status:'current'},engine:{channel:'stable'},diagnostics:{checks:[]}},stopFails:true};
window.fetch=async(url,opts={})=>{
 const q=window.__qa,path=String(url),body=typeof opts.body==='string'?JSON.parse(opts.body):opts.body;
 q.calls.push({path,body:body instanceof FormData?'file':body});let out={ok:true},status=200;
 if(path==='/api/state')out=q.state;
 else if(path==='/api/quality/status')out={engine_ready:true,engine_bundled:true};
 else if(path==='/api/quality/sources')out={sources:q.sources};
 else if(path==='/api/quality/upload'){const f=body.get('file');out={id:'source-'+(q.sources.length+1),name:f.name,kind:'image',width:12,height:10};q.sources.push(out);status=201;}
 else if(path==='/api/quality/start'){const source=q.sources.find(s=>s.id===body.source),id='job-'+(q.state.jobs.length+1);q.state.jobs.unshift({id,task:'quality',title:source.name,status:'queued',quality:{source,options:body.options,preview:body.preview},time:'test',detail:'queued'});out={id};status=202;}
 else if(path==='/api/cancel-download'){const j=q.state.jobs.find(j=>j.id===body.id);j.status='cancelled';j.detail='cancelled';}
 else if(path==='/api/quality/release')q.sources=q.sources.filter(s=>s.id!==body.id);
 else if(path==='/api/workspace/status')out={active:2,stopping:false};
 else if(path==='/api/workspace/heartbeat')out={sequence:0};
 else if(path==='/api/stop'&&q.stopFails){status=500;out={error:'fixture failure'};}
 else if(path==='/api/browser-link/status')out={paired:false};
 return new Response(JSON.stringify(out),{status,headers:{'Content-Type':'application/json'}});
};
Object.defineProperty(navigator,'sendBeacon',{value:()=>true,configurable:true});
'''
with sync_playwright() as p:
 b=p.chromium.launch(executable_path=os.getenv('CLIPCATCH_CHROMIUM')or shutil.which('chromium'),headless=True)
 page=b.new_page(viewport={'width':1200,'height':1000});errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.evaluate(mock)
 page.set_content((W/'index.html').read_text(encoding='utf-8').replace('__APP_VERSION__','1.9.6'))
 for name in ['premiere.js','quality.js','workspace-tabs.js','startup-update.js','lifecycle.js']:page.add_script_tag(content=(W/name).read_text(encoding='utf-8'))
 page.locator('#navQuality').click()
 with page.expect_file_chooser() as event:page.locator('#qualityChoose').click()
 chooser=event.value;assert chooser.is_multiple
 chooser.set_files([{'name':'one.png','mimeType':'image/png','buffer':b'fixture'},{'name':'two.png','mimeType':'image/png','buffer':b'fixture'}]);expect(page.locator('.q-item')).to_have_count(2);expect(page.locator('#qualityPreview')).to_be_enabled()
 page.locator('#qualityDrop').focus()
 page.evaluate("""()=>{const dt=new DataTransfer();dt.items.add(new File(['fixture'],'paste.png',{type:'image/png'}));document.querySelector('#qualityDrop').dispatchEvent(new ClipboardEvent('paste',{bubbles:true,cancelable:true,clipboardData:dt}));}""");expect(page.locator('.q-item')).to_have_count(3)
 page.evaluate("""()=>{const dt=new DataTransfer();dt.items.add(new File(['fixture'],'drop.png',{type:'image/png'}));document.querySelector('#qualityDrop').dispatchEvent(new DragEvent('drop',{bubbles:true,cancelable:true,dataTransfer:dt}));}""");expect(page.locator('.q-item')).to_have_count(4)
 page.locator('#qualityPreview').click();page.wait_for_function('window.__qa.state.jobs.length===4');assert page.evaluate("window.__qa.calls.filter(x=>x.path==='/api/quality/choose').length")==0
 page.evaluate('render(window.__qa.state)');page.locator('.q-actions button').filter(has_text='\ucde8\uc18c').first.click();page.wait_for_function("window.__qa.state.jobs.some(j=>j.status==='cancelled')");page.evaluate('render(window.__qa.state)');expect(page.locator('.q-actions button').filter(has_text='\ub2e4\uc2dc \uc2dc\ub3c4').first).to_be_visible()
 page.screenshot(path=str(out/'batch-workspace-desktop.png'),full_page=True);page.set_viewport_size({'width':390,'height':844});assert page.evaluate('document.documentElement.scrollWidth<=innerWidth');page.screenshot(path=str(out/'batch-workspace-narrow.png'),full_page=True)
 page.on('dialog',lambda d:d.accept());page.locator('#quitBtn').click();expect(page.locator('#quitStatus')).not_to_be_empty();expect(page.locator('#downloadBtn')).to_have_count(1);assert not errors,errors
 result={'file_picker_event':True,'multiple_files':True,'synthetic_paste':True,'synthetic_drop':True,'queue_cancel_retry':True,'failed_shutdown_not_claimed_successful':True,'javascript_errors':errors}
 (out/'browser-checks.json').write_text(json.dumps(result,indent=2),encoding='utf-8');print(json.dumps(result));b.close()

# Existing browser regression entrypoint also checks the actual folder button.
import runpy
runpy.run_path(str(R/"tests/folder_open_ui.py"),run_name="__main__")
