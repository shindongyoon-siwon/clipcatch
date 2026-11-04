export function attachClipPopup({message,selection,notice,refresh}) {
 const box=document.createElement('section');box.id='sectionTools';
 const field=(prefix,label)=>`<div class="clip-time" role="group" aria-label="${label} 시간"><span class="clip-time-title">${label}</span>${[['H','시',9999,'1'],['M','분',59,'1'],['S','초',59.999,'0.001']].map(([id,unit,max,step])=>`<label><input id="${prefix}${id}" type="number" min="0" max="${max}" step="${step}" inputmode="${id==='S'?'decimal':'numeric'}" value="0" placeholder="00" aria-label="${label} ${unit}" disabled><span>${unit}</span></label>`).join('')}</div>`;

 box.innerHTML=`<div class="clip-heading"><strong>\ud654\uc9c8\u00b7\uad6c\uac04 \uc120\ud0dd</strong></div><div class="clip-quality-row"><label for="sectionQuality">\ud654\uc9c8</label><select id="sectionQuality"><option value="0">\ucd5c\uace0\ud654\uc9c8</option></select></div>
 <div class="clip-section-row"><label class="clip-toggle"><input id="sectionOnly" type="checkbox" aria-controls="sectionTimes" disabled><span>\uad6c\uac04\ub9cc \uc800\uc7a5</span></label>
 <div id="sectionTimes" class="clip-times" data-enabled="false">${field('sectionStart','\uc2dc\uc791')}<span class="clip-time-arrow" aria-hidden="true">\u2192</span>${field('sectionEnd','\uc885\ub8cc')}</div></div>
 <p id="sectionInfo" role="status" aria-live="polite">\uc601\uc0c1 \ud55c \uac1c\ub97c \uc120\ud0dd\ud558\uba74 \ud655\uc778\ud574\uc694.</p><button id="sectionAgain" type="button">\ub2e4\uc2dc \ud655\uc778</button><button id="sectionSave" type="button" disabled>\uc120\ud0dd \uc124\uc815\uc73c\ub85c \uc800\uc7a5</button>`;
 document.getElementById('send').before(box);
 const css=document.createElement('style');css.textContent=`
 #sectionTools{margin:14px 0;padding:16px 18px;border:1px solid #344252;border-radius:12px;background:#141c25}
 #sectionTools .clip-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:14px}
 #sectionTools .clip-heading strong{font-size:14px;color:#eef3f7}#sectionTools .clip-heading small{font-size:10px;color:#8d9aab}
 #sectionTools .clip-quality-row{display:flex;align-items:center;gap:18px;padding-bottom:13px;border-bottom:1px solid #2e3a49}
 #sectionTools .clip-quality-row>label{font-size:13px;font-weight:650;color:#cbd5e1}
 #sectionTools select{background:#0f1720;color:#e9f1f5;border:1px solid #475665;border-radius:7px;padding:8px 12px;min-width:156px;font:inherit;font-size:13px}
 #sectionTools .clip-section-row{display:flex;align-items:center;gap:18px;flex-wrap:wrap;padding-top:14px}
 #sectionTools .clip-toggle{display:inline-flex;align-items:center;gap:7px;font-size:13px;font-weight:650;color:#dbe4ec;cursor:pointer;white-space:nowrap;flex-shrink:0}
 #sectionTools .clip-toggle input{accent-color:#d9ff57;width:16px;height:16px;margin:0}
 #sectionTools .clip-toggle:has(input:disabled){color:#8492a3;cursor:not-allowed}
 #sectionTools .clip-times{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
 #sectionTools .clip-time{display:flex;align-items:center;gap:6px;white-space:nowrap}
 #sectionTools .clip-time-title{font-size:12px;color:#b9c6d4;margin-right:3px}
 #sectionTools .clip-time label{display:inline-flex;align-items:center;gap:3px;font-size:11px;color:#91a1b4}
 #sectionTools .clip-time input{box-sizing:border-box;width:44px;padding:8px 3px;border:1px solid #465568;border-radius:6px;text-align:center;background:#0b131d;color:#e9f1f5;font-size:13px;font-variant-numeric:tabular-nums;appearance:textfield;-moz-appearance:textfield}
 #sectionTools .clip-time input[id$=S]{width:58px}#sectionTools .clip-time input::-webkit-inner-spin-button,#sectionTools .clip-time input::-webkit-outer-spin-button{-webkit-appearance:none;margin:0}
 #sectionTools .clip-time input:focus{outline:2px solid #d9ff57;outline-offset:2px}#sectionTools .clip-time input:disabled{background:#19222e;border-color:#293747;color:#657588;cursor:not-allowed}
 #sectionTools .clip-times[data-enabled=false] .clip-time-title,#sectionTools .clip-times[data-enabled=false] label,#sectionTools .clip-time-arrow{color:#728196}
 #sectionTools .clip-status-row{display:flex;align-items:flex-start;gap:12px;justify-content:space-between;margin-top:13px}
 #sectionTools #clipInfo{margin:0;color:#bac8d8;font-size:12px;line-height:1.65;overflow-wrap:anywhere;flex:1;min-width:0}
 #sectionAgain{background:transparent;color:#b4c4d7;border:0;padding:2px 0;border-radius:4px;cursor:pointer;font-size:11px;white-space:nowrap}
 #sectionTools .clip-footnote{margin:7px 0 0;color:#8594a7;font-size:11px;line-height:1.6}
 @media(max-width:680px){#sectionTools{padding:14px}#sectionTools .clip-section-row{gap:12px;align-items:flex-start;flex-direction:column}#sectionTools .clip-times{gap:10px}#sectionTools .clip-heading{align-items:flex-start}#sectionTools .clip-heading small{font-size:9px}}
 @media(max-width:510px){#sectionTools .clip-times{flex-direction:column;align-items:flex-start}#sectionTools .clip-time-arrow{display:none}#sectionTools .clip-time input{width:44px}#sectionTools .clip-time input[id$=S]{width:62px}}
 `;document.head.append(css);

 const $=id=>document.getElementById(id);let plan=null,group='',key='',sequence=0,timer=null,checking=false;
 const status=text=>{$('sectionInfo').textContent=text};
 const target=()=>group?{group_id:group}:{key:selection()?.key||''};
 function sync(){
  const enabled=$('sectionOnly').checked&&!checking&&plan?.partial==='supported';
  $('sectionTimes').dataset.enabled=String(enabled);
  $('sectionTimes').querySelectorAll('input').forEach(el=>el.disabled=!enabled);
  $('sectionOnly').disabled=!$('sectionOnly').checked&&(checking||plan?.partial!=='supported');
 }
 function time(prefix){
  const fields=['H','M','S'].map(k=>$(prefix+k));if(fields.some(el=>el.validity.badInput))return NaN;
  const raw=fields.map(el=>el.value.trim());
  if(!/^\d{1,4}$/.test(raw[0])||!/^\d{1,2}$/.test(raw[1])||!/^\d{1,2}(?:\.\d{1,3})?$/.test(raw[2]))return NaN;
  const [h,m,s]=raw.map(Number);if(m>=60||s>=60)return NaN;
  return (h*3600000+m*60000+Math.round(s*1000))/1000;
 }
 $('sectionTimes').querySelectorAll('input').forEach(el=>{
  el.addEventListener('keydown',e=>{if(['e','E','+','-',':'].includes(e.key))e.preventDefault();});
  el.addEventListener('paste',e=>{const v=e.clipboardData.getData('text').trim();if(!(el.id.endsWith('S')?/^\d{1,2}(?:\.\d{1,3})?$/:/^\d+$/).test(v))e.preventDefault();});
 });
 async function check(reuse=false){const t=target(),seq=++sequence,old=plan,requestedQuality=Number($('sectionQuality').value);plan=null;checking=true;$('sectionSave').disabled=true;sync();
  if(!t.key&&!t.group_id){checking=false;sync();return;}status('\ubd80\ubd84 \uc694\uccad \ud655\uc778 \uc911...');
  try{const p=await message({kind:'clip-preview',...t,silent:!!document.getElementById('silent')?.checked,quality:requestedQuality,token:reuse?old?.token||'':''});if(seq!==sequence)return;plan=p;checking=false;
   if(!reuse){$('sectionQuality').replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));for(const q of p.qualities||[])$('sectionQuality').add(new Option(q+'p',q));if(requestedQuality){const menu=$('sectionQuality');if(![...menu.options].some(o=>o.value===String(requestedQuality)))menu.add(new Option(requestedQuality+'p',requestedQuality));menu.value=String(requestedQuality);}}
   status(p.notice);sync();$('sectionSave').disabled=!p.token;
  }catch(e){if(seq===sequence){checking=false;sync();status(e.message);}}}
 $('sectionQuality').onchange=()=>check(true);$('sectionAgain').onclick=()=>check();
 $('sectionOnly').onchange=()=>{sync();if($('sectionOnly').checked&&!$('sectionStartM').disabled)$('sectionStartM').focus();};
 $('sectionSave').onclick=async()=>{if(!plan)return;const partial=$('sectionOnly').checked;if(partial&&plan.partial!=='supported'){status(plan.notice);return;}
  const a=time('sectionStart'),b=time('sectionEnd');
  if(partial&&(!Number.isFinite(a)||!Number.isFinite(b)||b<=a||b>plan.duration||b-a>=plan.duration)){status('\uc2dc\uc791\u00b7\uc885\ub8cc \uc2dc\uac04\uc744 \uc601\uc0c1 \uae38\uc774 \uc548\uc5d0\uc11c \uc785\ub825\ud574 \uc8fc\uc138\uc694.');return;}
  $('sectionSave').disabled=true;try{await message({kind:'clip-queue',token:plan.token,quality:plan.quality,partial,start:partial?String(a):'',end:partial?String(b):'',output_mode:$('outputMode').value,allow_duplicates:false});notice('\uc120\ud0dd\ud55c \uc124\uc815\uc73c\ub85c \uc800\uc7a5 \ub300\uae30\uc5d0 \ucd94\uac00\ud588\uc5b4\uc694.');refresh();}catch(e){status(e.message);}finally{$('sectionSave').disabled=false;}};
 function changed(){const current=selection()?.key||'';if(current===key)return;key=current;group='';sequence++;clearTimeout(timer);plan=null;checking=false;sync();$('sectionSave').disabled=true;timer=setTimeout(()=>check(),500);}
 new MutationObserver(changed).observe($('list'),{childList:true});$('list').addEventListener('change',changed);$('selectAll').addEventListener('change',changed);
 window.clipPopupInspectGroup=id=>{group=id;check();box.scrollIntoView({block:'nearest'});};
 // Preserve the no-full-download guard while section-only is selected.
 $('send').addEventListener('click',e=>{if($('sectionOnly').checked||$('sectionQuality').value!=='0'){e.preventDefault();e.stopImmediatePropagation();$('sectionSave').click();}},true);
 sync();changed();
}
