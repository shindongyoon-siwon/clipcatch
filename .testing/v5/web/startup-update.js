(() => {
 'use strict';
 const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text)n.textContent=text;if(cls)n.className=cls;return n;};
 const css=el('style');css.textContent=`.update-wall{border:1px solid #685a95;border-radius:22px;padding:34px;width:min(600px,92vw);max-height:90vh;color:#ecf1f6;background:#161d28;box-shadow:0 28px 100px #000a}.update-wall::backdrop{background:#080b13d9;backdrop-filter:blur(6px)}.update-wall h1{font:700 30px/1.35 inherit;margin:12px 0 18px}.update-wall p{color:#b9c6d6;line-height:1.65;font-size:14px;white-space:pre-line}.update-wall .label{font-size:12px;letter-spacing:2px;color:#c8a6ff}.update-wall .version{font-size:16px;color:#fff}.update-wall .primary{background:#c9adff;color:#151220;width:100%;border:0;border-radius:12px;font-size:17px;font-weight:700;padding:16px;margin-top:18px;cursor:pointer}.update-wall button:disabled{opacity:.5;cursor:default}.update-wall .minor{border:1px solid #56617a;color:#d2daea;background:transparent;border-radius:9px;padding:10px;margin:12px 6px 0 0;cursor:pointer}.update-wall progress{width:100%;height:10px;accent-color:#c9adff}.update-wall .error{color:#ffb8b8}.update-wall .live-job{font-size:12px;border-bottom:1px solid #354052;padding:6px 0}.update-wall .live-job button{float:right;padding:3px 7px;margin:0}`;document.head.append(css);
 const d=el('dialog',null,'update-wall');d.id='requiredUpdateDialog';d.setAttribute('aria-labelledby','updateWallTitle');
 const tag=el('div','CLIPCATCH TEST2 UPDATE','label'),title=el('h1','새 버전으로 업데이트해 주세요.');title.id='updateWallTitle';
 const version=el('div','','version'),detail=el('p'),bar=el('progress');bar.max=100;bar.hidden=true;
 const live=el('div'),error=el('p','','error'),install=el('button','업데이트 후 다시 시작','primary');install.id='requiredUpdateInstall';install.type='button';
 const retry=el('button','다시 확인','minor'),quit=el('button','종료','minor');retry.type=quit.type='button';
 d.append(tag,title,version,detail,bar,live,error,install,retry,quit);document.body.append(d);
 let request=false,previousFocus=null;
 d.addEventListener('cancel',e=>e.preventDefault());
 install.onclick=async()=>{request=true;error.textContent='';install.disabled=true;try{await post('/api/install-app-update');await poll();}catch(e){error.textContent=e.message;}finally{request=false;if(lastState)draw(lastState);}};
 retry.onclick=async()=>{retry.disabled=true;error.textContent='';try{await post('/api/check-app-update');await poll();}catch(e){error.textContent=e.message;}finally{retry.disabled=false;}};
 quit.onclick=()=>$('quitBtn').click();
 function draw(st){
  const u=st.app_update||{},status=u.status,required=!!st.update_required||['available','downloading','installing'].includes(status);
  if(!required){if(d.open){d.close();previousFocus?.focus?.();}return;}
  const active=(st.jobs||[]).filter(j=>['queued','downloading'].includes(j.status)),busy=active.length>0;
  version.textContent=(st.version||uiVersion)+' → '+(u.latest||'새 버전');
  title.textContent=status==='installing'?'업데이트를 적용하고 있습니다.':'새 버전으로 업데이트해 주세요.';
  detail.textContent=busy?'진행 중인 '+active.length+'개 작업은 계속됩니다.\n작업이 끝나면 업데이트할 수 있습니다.':status==='installing'?'새 버전 실행을 확인한 뒤 자동으로 열립니다.':'업데이트 후 이용할 수 있습니다.\n저장한 영상과 설정은 유지됩니다.';
  install.disabled=request||busy||!['available','error'].includes(status);
  install.textContent=busy?'진행 중인 작업 완료 대기':status==='downloading'?'업데이트 다운로드 '+Math.round(u.percent||0)+'%':status==='installing'?'교체 및 재시작 중':'업데이트 후 다시 시작';
  bar.hidden=!['downloading','installing'].includes(status);bar.value=u.percent||0;
  retry.hidden=['downloading','installing'].includes(status);quit.disabled=['downloading','installing'].includes(status);
  if(status==='error')error.textContent=u.detail||'업데이트를 다시 시도해 주세요.';
  live.replaceChildren();for(const j of active.slice(0,5)){const row=el('div',(j.title||'작업')+' · '+Math.round(j.percent||0)+'%','live-job');const cancel=el('button','취소','minor');cancel.onclick=async()=>{if(!confirm('선택한 작업을 취소하시겠습니까?'))return;try{await post('/api/cancel-download',{id:j.id});await poll();}catch(e){error.textContent=e.message;}};row.append(cancel);live.append(row);}
  if(!d.open){previousFocus=document.activeElement;d.showModal();install.focus();}
 }
 const previous=render;render=function(st){previous(st);if(st)draw(st);};if(lastState)draw(lastState);
})();
