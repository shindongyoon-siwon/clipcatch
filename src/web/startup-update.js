(() => {
 'use strict';
 const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text)n.textContent=text;if(cls)n.className=cls;return n;};
 const css=el('style');css.textContent=`.update-wall{border:1px solid #576777;border-radius:18px;padding:28px;width:min(900px,94vw);max-height:90vh;overflow:auto;color:#ecf1f6;background:#161d28;box-shadow:0 28px 100px #000a}.update-wall::backdrop{background:#080b13d9;backdrop-filter:blur(5px)}.update-wall h1{font-size:25px;line-height:1.35;margin:12px 0 18px}.update-wall p{color:#b9c6d6;line-height:1.65;font-size:14px;white-space:pre-line}.update-wall .label{font-size:12px;letter-spacing:1px;color:#d9ff57}.update-wall .version{font-size:16px;color:#fff}.update-wall .primary{background:#d9ff57;color:#151d12;width:100%;border:0;border-radius:10px;font-size:16px;font-weight:700;padding:15px;margin-top:16px;cursor:pointer}.update-wall button:disabled{opacity:.5;cursor:default}.update-wall .minor{border:1px solid #56617a;color:#d2daea;background:transparent;border-radius:8px;padding:10px;margin:12px 6px 0 0;cursor:pointer}.update-wall progress{width:100%;height:10px;accent-color:#d9ff57}.update-wall .error{color:#ffb8b8}.update-wall .live-job{font-size:12px;border-bottom:1px solid #354052;padding:8px 0}.update-wall .live-job button{float:right;padding:3px 7px;margin:0}.update-columns{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:24px}.update-notes{background:#202b37;border:1px solid #3a4b5d;border-radius:12px;padding:18px;min-width:0}.update-notes h2{font-size:17px;margin:0 0 14px}.update-notes pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px;font-family:inherit;line-height:1.8;margin:0;max-height:47vh;overflow:auto;color:#ced8e2}.update-notes a{display:block;font-size:12px;color:#d9ff57;margin-top:16px}.update-wall [hidden]{display:none!important}@media(max-width:680px){.update-columns{grid-template-columns:1fr}.update-wall{padding:20px}.update-notes pre{max-height:27vh}}`;document.head.append(css);
 const d=el('dialog',null,'update-wall');d.id='requiredUpdateDialog';d.setAttribute('aria-labelledby','updateWallTitle');
 const tag=el('div','CLIPCATCH UPDATE','label'),title=el('h1','새 버전을 확인하고 있어요.');title.id='updateWallTitle';
 const version=el('div','','version'),detail=el('p'),bar=el('progress');bar.max=100;bar.hidden=true;
 const live=el('div'),error=el('p','','error'),install=el('button','업데이트 후 다시 시작','primary');install.id='requiredUpdateInstall';install.type='button';
 const retry=el('button','다시 확인','minor'),quit=el('button','종료','minor');retry.type=quit.type='button';
 const left=el('section'),right=el('section',null,'update-notes'),notes=el('pre'),link=el('a','전체 업데이트 안내');notes.id='updateReleaseNotes';link.target='_blank';link.rel='noopener noreferrer';link.hidden=true;
 right.append(el('h2','이번 업데이트 안내'),notes,link);left.append(version,detail,bar,live,error,install,retry,quit);
 const columns=el('div',null,'update-columns');columns.append(left,right);d.append(tag,title,columns);document.body.append(d);
 let request=false,previousFocus=null;
 d.addEventListener('cancel',e=>e.preventDefault());
 install.onclick=async()=>{request=true;error.textContent='';install.disabled=true;try{await post('/api/install-app-update');await poll();}catch(e){error.textContent=e.message;}finally{request=false;if(lastState)draw(lastState);}};
 retry.onclick=async()=>{retry.disabled=true;error.textContent='';try{await post('/api/check-app-update');await poll();}catch(e){error.textContent=e.message;}finally{retry.disabled=false;}};
 quit.onclick=()=>$('quitBtn').click();
 function draw(st){
  const u=st.app_update||{},status=u.status,checking=!!st.update_checking,required=!!st.update_required||['available','downloading','installing'].includes(status);
  if(!required&&!checking){if(d.open){d.close();previousFocus?.focus?.();}return;}
  const active=(st.jobs||[]).filter(j=>['queued','downloading'].includes(j.status)),busy=active.length>0||Number(st.active_tasks)>0;
  version.textContent=(st.channel==='test'?'TEST2 테스트판 ':'ClipCatch ')+(st.version||uiVersion)+(u.latest?' → '+u.latest:'');
  title.textContent=checking&&!required?'새 버전을 확인하고 있어요.':status==='installing'?'업데이트를 적용하고 있어요.':'업데이트 후 이용해 주세요.';
  detail.textContent=checking&&!required?'사용 전에 공식 최신 버전을 확인해요.':busy?'진행 중인 작업은 계속됩니다.\n작업이 끝나면 업데이트할 수 있어요.':status==='installing'?'새 버전 실행을 확인한 뒤 자동으로 열립니다.':'새 다운로드·변환·AI 작업은 업데이트 완료 후 시작할 수 있어요.\n저장한 영상과 설정은 유지돼요.';
  // Never render release Markdown/HTML as markup. No remote images or scripts.
  notes.textContent=u.notes|| (checking?'변경사항을 확인하고 있어요.':'이 버전에 등록된 안내사항이 없어요. 아래 공식 배포 페이지를 확인해 주세요.');
  const validURL=typeof u.release_url==='string'&&/^https:\/\/github\.com\/shindongyoon-siwon\/clipcatch\/releases\/tag\/(v[0-9]+\.[0-9]+\.[0-9]+|test2-v[0-9]+\.[0-9]+\.[0-9]+-test\.[0-9]+)$/.test(u.release_url);
  link.hidden=!validURL;if(validURL)link.href=u.release_url;else link.removeAttribute('href');
  install.disabled=request||busy||checking||!required||!['available','error'].includes(status);
  install.textContent=busy?'진행 중인 작업 완료 대기':status==='downloading'?'업데이트 다운로드 '+Math.round(u.percent||0)+'%':status==='installing'?'교체 및 재시작 중':'업데이트 후 다시 시작';
  bar.hidden=!['downloading','installing'].includes(status);bar.value=u.percent||0;
  retry.hidden=checking||['downloading','installing'].includes(status);quit.disabled=['downloading','installing'].includes(status);
  if(status==='error')error.textContent=u.detail||'업데이트를 다시 시도해 주세요.';else if(!request)error.textContent='';
  live.replaceChildren();for(const j of active.slice(0,5)){const row=el('div',(j.title||'작업')+' · '+Math.round(j.percent||0)+'%','live-job');const cancel=el('button','취소','minor');cancel.onclick=async()=>{try{await post('/api/cancel-download',{id:j.id});await poll();}catch(e){error.textContent=e.message;}};row.append(cancel);live.append(row);}
  if(!d.open){previousFocus=document.activeElement;d.showModal();(install.disabled?quit:install).focus();}
 }
 const previous=render;render=function(st){previous(st);if(st)draw(st);};if(lastState)draw(lastState);

 // A new tab or a return to this page should discover releases without a manual click.
 // The app applies the shared freshness/concurrency guard across all open tabs.
 let automaticTimer=null,automaticRequest=false,lastAutomaticCheckAt=-Infinity;
 const canCheckAutomatically=()=>document.visibilityState!=='hidden'&&!['checking','downloading','installing'].includes(lastState?.app_update?.status);
 function scheduleAutomaticCheck(){
  if(!canCheckAutomatically()||automaticRequest||automaticTimer!==null)return;
  const delay=Math.max(120,1000-(performance.now()-lastAutomaticCheckAt));
  automaticTimer=setTimeout(async()=>{
   automaticTimer=null;if(!canCheckAutomatically()||automaticRequest)return;
   automaticRequest=true;lastAutomaticCheckAt=performance.now();
   const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),10000);
   // Show any release the app has already found while the fresh check runs.
   void poll();
   try{
    await fetch('/api/check-app-update',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({automatic:true}),cache:'no-store',signal:controller.signal});
   }catch(_){
    // A failed background request is not evidence of a required update.
   }finally{
    clearTimeout(timeout);automaticRequest=false;void poll();
   }
  },delay);
 }
 window.addEventListener('focus',scheduleAutomaticCheck);
 window.addEventListener('pageshow',scheduleAutomaticCheck);
 window.addEventListener('online',scheduleAutomaticCheck);
 document.addEventListener('visibilitychange',scheduleAutomaticCheck);
 window.addEventListener('pagehide',()=>{if(automaticTimer!==null){clearTimeout(automaticTimer);automaticTimer=null;}});
 scheduleAutomaticCheck();
})();
