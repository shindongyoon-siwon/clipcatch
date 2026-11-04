/* Section candidate UI. No preview player and no background video download. */
(() => {
 'use strict';
 const input=document.getElementById('urlInput'),button=document.getElementById('downloadBtn');
 if(!input||!button)return;
 const box=document.createElement('section');box.id='clipControls';box.className='reddit-auth';
 document.title='ClipCatch SECTION TEST 2.1.0-test.1';
 box.innerHTML=`<strong style="display:block;margin-bottom:10px;color:#d9ff57">SECTION TEST 2.1.0-test.1</strong><div style="display:flex;gap:18px;align-items:center;flex-wrap:wrap">
 <label>\ud654\uc9c8 <select id="clipQuality"><option value="0">\ucd5c\uace0\ud654\uc9c8</option></select></label>
 <label><input type="checkbox" id="clipPartial"> \uad6c\uac04\ub9cc \uc800\uc7a5</label></div>
 <div id="clipTimes" hidden style="margin-top:12px;display:flex;gap:10px;align-items:center;flex-wrap:wrap">
 <label>\uc2dc\uc791 <input id="clipStart" type="text" placeholder="00:30.000" inputmode="decimal" style="width:104px"></label>
 <label>\uc885\ub8cc <input id="clipEnd" type="text" placeholder="00:40.000" inputmode="decimal" style="width:104px"></label>
 <span>\uc785\ub825 \uad6c\uac04\ub9cc MP4 \uc800\uc7a5</span></div>
 <p id="clipInfo" role="status" aria-live="polite">\ub9c1\ud06c \ud55c \uac1c\ub97c \ub123\uc73c\uba74 \ud654\uc9c8\u00b7\uad6c\uac04 \uc800\uc7a5 \uac00\ub2a5 \uc5ec\ubd80\ub97c \ud655\uc778\ud574\uc694.</p>
 <button type="button" id="clipRefresh" class="plain-btn">\ub2e4\uc2dc \ud655\uc778</button>
 <p style="margin-bottom:0">\uc804\uccb4 \ub2e4\uc6b4\ub85c\ub4dc\ub85c \uc790\ub3d9 \uc804\ud658\ud558\uc9c0 \uc54a\uc544\uc694. \uc815\ud655\ud55c \uad6c\uac04 \uc800\uc7a5\uc740 \uc7ac\uc555\ucd95\uc774 \ud544\uc694\ud560 \uc218 \uc788\uc5b4\uc694.</p>`;
 input.closest('.urlwrap').after(box);
 const css=document.createElement('style');css.textContent='#clipControls label{font-size:13px;color:#dbe4ec;display:inline-flex;align-items:center;gap:8px}#clipControls select,#clipControls input[type=text]{background:#111a22;color:#e9f1f5;border:1px solid #475665;border-radius:7px;padding:8px 10px;font-size:14px}#clipControls p{font-size:12px;line-height:1.6}#clipRefresh{background:#26333f;color:#dce8ef;border:1px solid #475665;padding:7px 12px;border-radius:7px;cursor:pointer}#clipControls input:disabled{cursor:not-allowed}';document.head.append(css);
 const $=id=>document.getElementById(id),q=$('clipQuality'),partial=$('clipPartial'),status=$('clipInfo');
 let seq=0,timer=null,controller=null,plan=null,lastURL='',checking=false,submitting=false;
 const links=()=>input.value.match(/https?:\/\/[^\s<>"']+/g)||[];
 function show(text){status.textContent=text;}
 function sync(){ $('clipTimes').hidden=!partial.checked; $('clipTimes').style.display=partial.checked?'flex':'none'; }
 async function post(path,data,signal){const r=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data),signal});const v=await r.json();if(!r.ok){const e=Error(v.error||'Request failed');e.duplicates=v.duplicates;throw e;}return v;}
 async function inspect(reuse=false){
  const urls=links(),id=++seq;controller?.abort();controller=new AbortController();
  if(urls.length!==1){plan=null;checking=false;partial.disabled=true;show(urls.length>1?'\ud654\uc9c8\u00b7\uad6c\uac04 \uc120\ud0dd\uc740 \ub9c1\ud06c \ud55c \uac1c\uc529 \ud655\uc778\ud574 \uc8fc\uc138\uc694. \uc5ec\ub7ec \ub9c1\ud06c\ub294 \uae30\uc874 \uc804\uccb4 \uc800\uc7a5\uc774 \uac00\ub2a5\ud574\uc694.':'\ub9c1\ud06c\ub97c \ub123\uc73c\uba74 \uc790\ub3d9\uc73c\ub85c \ud655\uc778\ud574\uc694.');return;}
  const url=urls[0];const token=reuse&&lastURL===url?plan?.token:'';lastURL=url;
  checking=true;partial.disabled=true;show('\uc601\uc0c1 \uc815\ubcf4\u00b7\ubd80\ubd84 \uc694\uccad \ud655\uc778 \uc911...');
  try{
   const data=await post('/api/clip-preview',{url,quality:Number(q.value),token:token||''},controller.signal);
   if(id!==seq||links()[0]!==url)return;plan=data;checking=false;
   if(!reuse){q.replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));for(const height of data.qualities||[])q.add(new Option(height+'p',String(height)));}
   partial.disabled=data.partial!=='supported';
   const duration=data.duration?' \u00b7 '+Math.floor(data.duration/60)+':'+String(Math.floor(data.duration%60)).padStart(2,'0'):'';
   show(data.notice+duration);
  }catch(e){if(e.name!=='AbortError'&&id===seq){checking=false;plan=null;partial.disabled=true;show(e.message);}}
 }
 function schedule(){q.replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));clearTimeout(timer);seq++;controller?.abort();plan=null;partial.disabled=true;timer=setTimeout(()=>inspect(),600);}
 input.addEventListener('input',schedule);$('pasteBtn')?.addEventListener('click',()=>setTimeout(schedule,100));
 q.addEventListener('change',()=>inspect(true));partial.addEventListener('change',sync);$('clipRefresh').onclick=()=>inspect(false);sync();
 function time(s){if(!/^\d+(?::\d{1,2}){0,2}(?:\.\d{1,3})?$/.test(s))return NaN;const p=s.split(':');if(p.slice(1).some(v=>Number(v)>=60))return NaN;return p.reduce((a,v)=>a*60+Number(v),0);}
 button.addEventListener('click',async ev=>{
  if(links().length!==1){if(partial.checked){ev.preventDefault();ev.stopImmediatePropagation();show('\uad6c\uac04 \uc800\uc7a5\uc740 \ub9c1\ud06c \ud55c \uac1c\ub9cc \uc120\ud0dd\ud574 \uc8fc\uc138\uc694.');}return;}
  if(!partial.checked && !plan && !checking && q.value==='0')return; // Explicit ordinary full download, never a partial fallback.
  ev.preventDefault();ev.stopImmediatePropagation();if(submitting)return;
  if(checking||!plan||lastURL!==links()[0]){show('\ud655\uc778\uc774 \ub05d\ub098\uba74 \uc2dc\uc791\ud574 \uc8fc\uc138\uc694.');return;}
  if(partial.checked && plan.partial!=='supported'){show(plan.notice);return;}
  const start=$('clipStart').value.trim(),end=$('clipEnd').value.trim(),a=time(start),b=time(end);
  if(partial.checked&&(!Number.isFinite(a)||!Number.isFinite(b)||a<0||b<=a||b>plan.duration||b-a>=plan.duration)){show('\uc2dc\uc791\u00b7\uc885\ub8cc \uc2dc\uac04\uc744 \uc601\uc0c1 \uae38\uc774 \uc548\uc5d0\uc11c \uc785\ub825\ud574 \uc8fc\uc138\uc694.');return;}
  submitting=true;
  try{
   const state=await (await fetch('/api/state')).json();
   const data={token:plan.token,quality:plan.quality,partial:partial.checked,start,end,output_mode:state.output_mode||'premiere',allow_duplicates:false};
   let result;try{result=await post('/api/clip-queue',data);}catch(e){if(!e.duplicates||!confirm('\uc774 \uad6c\uac04\u00b7\ud654\uc9c8\uc744 \uc774\ubbf8 \ubc1b\uc558\uc5b4\uc694. \uc0ac\ubcf8\uc73c\ub85c \uc800\uc7a5\ud560\uae4c\uc694?'))throw e;result=await post('/api/clip-queue',{...data,allow_duplicates:true});}
   show(partial.checked?'\uc9c0\uc815\ud55c \uad6c\uac04\ub9cc \uc800\uc7a5 \ub300\uae30\uc5d0 \ucd94\uac00\ud588\uc5b4\uc694.':'\uc120\ud0dd\ud55c \ud654\uc9c8\ub85c \ucd94\uac00\ud588\uc5b4\uc694.');
  }catch(e){show(e.message);}finally{submitting=false;}
 },true);
 // Keyboard shortcuts use the same guarded button path as a click.
 input.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key==='Enter'){e.preventDefault();e.stopImmediatePropagation();button.click();}},true);
 if(links().length)schedule();
})();
