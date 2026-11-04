/* Section candidate UI. No preview player and no background video download. */
(() => {
 'use strict';
 const input=document.getElementById('urlInput'),button=document.getElementById('downloadBtn');
 if(!input||!button)return;
 const box=document.createElement('section');box.id='clipControls';box.className='reddit-auth';
 document.title='ClipCatch SECTION TEST 2.1.0-test.2';
 const field=(prefix,label)=>`<div class="clip-time" role="group" aria-label="${label} 시간"><span class="clip-time-title">${label}</span>${[['H','시',9999,'1'],['M','분',59,'1'],['S','초',59.999,'0.001']].map(([id,unit,max,step])=>`<label><input id="${prefix}${id}" type="number" min="0" max="${max}" step="${step}" inputmode="${id==='S'?'decimal':'numeric'}" value="0" placeholder="00" aria-label="${label} ${unit}" disabled><span>${unit}</span></label>`).join('')}</div>`;
 box.innerHTML=`<div class="clip-heading"><strong>다운로드 설정</strong><small>SECTION TEST 2.1.0-test.2</small></div>
 <div class="clip-quality-row"><label for="clipQuality">화질</label><select id="clipQuality"><option value="0">최고화질</option></select></div>
 <div class="clip-section-row"><label class="clip-toggle"><input type="checkbox" id="clipPartial" aria-controls="clipTimes" disabled><span>구간만 저장</span></label>
 <div id="clipTimes" class="clip-times" data-enabled="false">${field('clipStart','시작')}<span class="clip-time-arrow" aria-hidden="true">→</span>${field('clipEnd','종료')}</div></div>
 <div class="clip-status-row"><p id="clipInfo" role="status" aria-live="polite">링크 한 개를 넣으면 화질과 구간 저장 가능 여부를 확인해요.</p><button type="button" id="clipRefresh" class="plain-btn">다시 확인</button></div>
 <p class="clip-footnote">입력한 구간만 저장해요. 실패해도 전체 영상으로 전환하지 않아요.</p>`;
 input.closest('.urlwrap').after(box);
 const css=document.createElement('style');css.textContent=`
 #clipControls{margin:14px 0;padding:16px 18px;border:1px solid #344252;border-radius:12px;background:#141c25}
 #clipControls .clip-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:14px}
 #clipControls .clip-heading strong{font-size:14px;color:#eef3f7}#clipControls .clip-heading small{font-size:10px;color:#8d9aab}
 #clipControls .clip-quality-row{display:flex;align-items:center;gap:18px;padding-bottom:13px;border-bottom:1px solid #2e3a49}
 #clipControls .clip-quality-row>label{font-size:13px;font-weight:650;color:#cbd5e1}
 #clipControls select{background:#0f1720;color:#e9f1f5;border:1px solid #475665;border-radius:7px;padding:8px 12px;min-width:156px;font:inherit;font-size:13px}
 #clipControls .clip-section-row{display:flex;align-items:center;gap:18px;flex-wrap:wrap;padding-top:14px}
 #clipControls .clip-toggle{display:inline-flex;align-items:center;gap:7px;font-size:13px;font-weight:650;color:#dbe4ec;cursor:pointer;white-space:nowrap;flex-shrink:0}
 #clipControls .clip-toggle input{accent-color:#d9ff57;width:16px;height:16px;margin:0}
 #clipControls .clip-toggle:has(input:disabled){color:#8492a3;cursor:not-allowed}
 #clipControls .clip-times{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
 #clipControls .clip-time{display:flex;align-items:center;gap:6px;white-space:nowrap}
 #clipControls .clip-time-title{font-size:12px;color:#b9c6d4;margin-right:3px}
 #clipControls .clip-time label{display:inline-flex;align-items:center;gap:3px;font-size:11px;color:#91a1b4}
 #clipControls .clip-time input{box-sizing:border-box;width:44px;padding:8px 3px;border:1px solid #465568;border-radius:6px;text-align:center;background:#0b131d;color:#e9f1f5;font-size:13px;font-variant-numeric:tabular-nums;appearance:textfield;-moz-appearance:textfield}
 #clipControls .clip-time input[id$=S]{width:58px}#clipControls .clip-time input::-webkit-inner-spin-button,#clipControls .clip-time input::-webkit-outer-spin-button{-webkit-appearance:none;margin:0}
 #clipControls .clip-time input:focus{outline:2px solid #d9ff57;outline-offset:2px}#clipControls .clip-time input:disabled{background:#19222e;border-color:#293747;color:#657588;cursor:not-allowed}
 #clipControls .clip-times[data-enabled=false] .clip-time-title,#clipControls .clip-times[data-enabled=false] label,#clipControls .clip-time-arrow{color:#728196}
 #clipControls .clip-status-row{display:flex;align-items:flex-start;gap:12px;justify-content:space-between;margin-top:13px}
 #clipControls #clipInfo{margin:0;color:#bac8d8;font-size:12px;line-height:1.65;overflow-wrap:anywhere;flex:1;min-width:0}
 #clipRefresh{background:transparent;color:#b4c4d7;border:0;padding:2px 0;border-radius:4px;cursor:pointer;font-size:11px;white-space:nowrap}
 #clipControls .clip-footnote{margin:7px 0 0;color:#8594a7;font-size:11px;line-height:1.6}
 @media(max-width:680px){#clipControls{padding:14px}#clipControls .clip-section-row{gap:12px;align-items:flex-start;flex-direction:column}#clipControls .clip-times{gap:10px}#clipControls .clip-heading{align-items:flex-start}#clipControls .clip-heading small{font-size:9px}}
 @media(max-width:510px){#clipControls .clip-times{flex-direction:column;align-items:flex-start}#clipControls .clip-time-arrow{display:none}#clipControls .clip-time input{width:44px}#clipControls .clip-time input[id$=S]{width:62px}}
 `;document.head.append(css);
 const $=id=>document.getElementById(id),q=$('clipQuality'),partial=$('clipPartial'),status=$('clipInfo');
 let seq=0,timer=null,controller=null,plan=null,lastURL='',checking=false,submitting=false;
 const links=()=>input.value.match(/https?:\/\/[^\s<>"']+/g)||[];
 function show(text){status.textContent=text;}
 function sync(){
  const enabled=partial.checked&&!checking&&plan?.partial==='supported';
  $('clipTimes').dataset.enabled=String(enabled);
  $('clipTimes').querySelectorAll('input').forEach(el=>el.disabled=!enabled);
  // A selected section remains selected on failure: allow turning it OFF explicitly.
  partial.disabled=!partial.checked&&(checking||plan?.partial!=='supported');
 }
 function time(prefix){
  const vals=['H','M','S'].map(k=>$(prefix+k));
  if(vals.some(el=>el.validity.badInput))return NaN;
  const raw=vals.map(el=>el.value.trim());
  if(!/^\d{1,4}$/.test(raw[0])||!/^\d{1,2}$/.test(raw[1])||!/^\d{1,2}(?:\.\d{1,3})?$/.test(raw[2]))return NaN;
  const [h,m,sec]=raw.map(Number);if(m>=60||sec>=60)return NaN;
  return (h*3600000+m*60000+Math.round(sec*1000))/1000;
 }
 $('clipTimes').querySelectorAll('input').forEach(el=>{
  el.addEventListener('keydown',e=>{if(['e','E','+','-',':'].includes(e.key))e.preventDefault();});
  el.addEventListener('paste',e=>{const v=e.clipboardData.getData('text').trim();if(!(el.id.endsWith('S')?/^\d{1,2}(?:\.\d{1,3})?$/:/^\d+$/).test(v))e.preventDefault();});
 });
 async function post(path,data,signal){const r=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data),signal});const v=await r.json();if(!r.ok){const e=Error(v.error||'Request failed');e.duplicates=v.duplicates;throw e;}return v;}
 async function inspect(reuse=false){
  const urls=links(),id=++seq;controller?.abort();controller=new AbortController();
  if(urls.length!==1){plan=null;checking=false;sync();show(urls.length>1?'\ud654\uc9c8\u00b7\uad6c\uac04 \uc120\ud0dd\uc740 \ub9c1\ud06c \ud55c \uac1c\uc529 \ud655\uc778\ud574 \uc8fc\uc138\uc694. \uc5ec\ub7ec \ub9c1\ud06c\ub294 \uae30\uc874 \uc804\uccb4 \uc800\uc7a5\uc774 \uac00\ub2a5\ud574\uc694.':'\ub9c1\ud06c\ub97c \ub123\uc73c\uba74 \uc790\ub3d9\uc73c\ub85c \ud655\uc778\ud574\uc694.');return;}
  const url=urls[0];const token=reuse&&lastURL===url?plan?.token:'';lastURL=url;
  checking=true;sync();show('\uc601\uc0c1 \uc815\ubcf4\u00b7\ubd80\ubd84 \uc694\uccad \ud655\uc778 \uc911...');
  try{
   const data=await post('/api/clip-preview',{url,quality:Number(q.value),token:token||''},controller.signal);
   if(id!==seq||links()[0]!==url)return;plan=data;checking=false;
   if(!reuse){q.replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));for(const height of data.qualities||[])q.add(new Option(height+'p',String(height)));}
   sync();
   const duration=data.duration?' \u00b7 '+Math.floor(data.duration/60)+':'+String(Math.floor(data.duration%60)).padStart(2,'0'):'';
   show(data.notice+duration);
  }catch(e){if(e.name!=='AbortError'&&id===seq){checking=false;plan=null;sync();show(e.message);}}
 }
 function schedule(){q.replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));clearTimeout(timer);seq++;controller?.abort();plan=null;checking=false;sync();timer=setTimeout(()=>inspect(),600);}
 input.addEventListener('input',schedule);$('pasteBtn')?.addEventListener('click',()=>setTimeout(schedule,100));
 q.addEventListener('change',()=>inspect(true));partial.addEventListener('change',()=>{sync();if(partial.checked&&!$('clipStartM').disabled)$('clipStartM').focus();});$('clipRefresh').onclick=()=>inspect(false);sync();
 button.addEventListener('click',async ev=>{
  if(links().length!==1){if(partial.checked){ev.preventDefault();ev.stopImmediatePropagation();show('\uad6c\uac04 \uc800\uc7a5\uc740 \ub9c1\ud06c \ud55c \uac1c\ub9cc \uc120\ud0dd\ud574 \uc8fc\uc138\uc694.');}return;}
  if(!partial.checked && !plan && !checking && q.value==='0')return; // Explicit ordinary full download, never a partial fallback.
  ev.preventDefault();ev.stopImmediatePropagation();if(submitting)return;
  if(checking||!plan||lastURL!==links()[0]){show('\ud655\uc778\uc774 \ub05d\ub098\uba74 \uc2dc\uc791\ud574 \uc8fc\uc138\uc694.');return;}
  if(partial.checked && plan.partial!=='supported'){show(plan.notice);return;}
  const a=time('clipStart'),b=time('clipEnd'),start=partial.checked?String(a):'',end=partial.checked?String(b):'';
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
