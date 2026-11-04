export function attachClipPopup({message,selection,notice,refresh}) {
 const box=document.createElement('section');box.id='sectionTools';box.style.cssText='margin:12px 0;padding:10px;border:1px solid #657052;border-radius:8px';
 box.innerHTML=`<strong>\ud654\uc9c8\u00b7\uad6c\uac04 \uc120\ud0dd</strong><div style="margin:8px 0"><select id="sectionQuality"><option value="0">\ucd5c\uace0\ud654\uc9c8</option></select> <label><input id="sectionOnly" type="checkbox">\uad6c\uac04\ub9cc</label></div><div id="sectionTimes" hidden><input id="sectionStart" placeholder="00:30" style="width:100px"> ~ <input id="sectionEnd" placeholder="00:40" style="width:100px"></div><p id="sectionInfo" role="status">\uc601\uc0c1 \ud55c \uac1c\ub97c \uc120\ud0dd\ud558\uba74 \ud655\uc778\ud574\uc694.</p><button id="sectionAgain" type="button">\ub2e4\uc2dc \ud655\uc778</button><button id="sectionSave" type="button" disabled>\uc120\ud0dd \uc124\uc815\uc73c\ub85c \uc800\uc7a5</button>`;
 document.getElementById('send').before(box);
 const $=id=>document.getElementById(id);let plan=null,group='',key='',sequence=0,timer=null;
 const status=text=>{$('sectionInfo').textContent=text};
 const target=()=>group?{group_id:group}:{key:selection()?.key||''};
 async function check(reuse=false){const t=target(),seq=++sequence;const old=plan;plan=null;$('sectionSave').disabled=true;$('sectionOnly').disabled=true;
  if(!t.key&&!t.group_id)return;status('\ubd80\ubd84 \uc694\uccad \ud655\uc778 \uc911...');
  try{const p=await message({kind:'clip-preview',...t,silent:!!document.getElementById('silent')?.checked,quality:Number($('sectionQuality').value),token:reuse?old?.token||'':''});if(seq!==sequence)return;plan=p;
   if(!reuse){$('sectionQuality').replaceChildren(new Option('\ucd5c\uace0\ud654\uc9c8','0'));for(const q of p.qualities||[])$('sectionQuality').add(new Option(q+'p',q));}
   status(p.notice);$('sectionOnly').disabled=p.partial!=='supported';$('sectionSave').disabled=!p.token;
  }catch(e){if(seq===sequence)status(e.message);}}
 $('sectionQuality').onchange=()=>check(true);$('sectionAgain').onclick=()=>check();
 $('sectionOnly').onchange=()=>{$('sectionTimes').hidden=!$('sectionOnly').checked};
 $('sectionSave').onclick=async()=>{if(!plan)return;const partial=$('sectionOnly').checked;if(partial&&plan.partial!=='supported'){status(plan.notice);return;}
  $('sectionSave').disabled=true;try{await message({kind:'clip-queue',token:plan.token,quality:plan.quality,partial,start:$('sectionStart').value.trim(),end:$('sectionEnd').value.trim(),output_mode:$('outputMode').value,allow_duplicates:false});notice('\uc120\ud0dd\ud55c \uc124\uc815\uc73c\ub85c \uc800\uc7a5 \ub300\uae30\uc5d0 \ucd94\uac00\ud588\uc5b4\uc694.');refresh();}catch(e){status(e.message);}finally{$('sectionSave').disabled=false;}};
 function changed(){const current=selection()?.key||'';if(current===key)return;key=current;group='';sequence++;clearTimeout(timer);timer=setTimeout(()=>check(),500);}
 new MutationObserver(changed).observe($('list'),{childList:true});$('list').addEventListener('change',changed);$('selectAll').addEventListener('change',changed);
 window.clipPopupInspectGroup=id=>{group=id;check();box.scrollIntoView({block:'nearest'});};
 // Never allow the ordinary full-download action while section-only is selected.
 $('send').addEventListener('click',e=>{if($('sectionOnly').checked||$('sectionQuality').value!=='0'){e.preventDefault();e.stopImmediatePropagation();$('sectionSave').click();}},true);
 changed();
}
