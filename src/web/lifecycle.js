(() => {
 'use strict';
 const id=Array.from(crypto.getRandomValues(new Uint8Array(16)),n=>n.toString(16).padStart(2,'0')).join('');
 let closing=false,closed=false;
 let connection=null;
 function connect(){
  if(closed||closing||connection)return;
  connection=new EventSource('/api/workspace/events?id='+id);
  connection.addEventListener('open-workspace',()=>{window.focus();document.title='ClipCatch';window.dispatchEvent(new Event('clipcatch-open'));});
 }
 connect();
 window.addEventListener('pageshow',()=>{closed=false;connect();});
 window.addEventListener('pagehide',()=>{closed=true;connection?.close();connection=null;});
 window.addEventListener('beforeunload',e=>{if(window.clipcatchHasUnsavedQuality?.()){e.preventDefault();e.returnValue='';}});
 const btn=document.getElementById('quitBtn');if(!btn)return;
 btn.textContent='\ud504\ub85c\uadf8\ub7a8 \uc885\ub8cc';btn.title='\ub9c8\uc9c0\ub9c9 \ud0ed\uc744 \ub2eb\uc73c\uba74 15\ucd08 \ud6c4 \uc790\ub3d9 \uc885\ub8cc\ud569\ub2c8\ub2e4. \uc791\uc5c5 \uc911\uc774\uba74 \uc644\ub8cc\ub97c \uae30\ub2e4\ub9bd\ub2c8\ub2e4.';
 const msg=document.createElement('p');msg.id='quitStatus';msg.setAttribute('role','status');msg.style.cssText='font-size:12px;color:#ffb6bd;text-align:right';btn.closest('.navbar').after(msg);
 btn.onclick=async()=>{
  if(closing)return;btn.disabled=true;msg.textContent='';
  try{
   const r=await fetch('/api/workspace/status',{cache:'no-store',signal:AbortSignal.timeout(4000)});if(!r.ok)throw Error();const state=await r.json();
   const text=state.active>0?`${state.active}\uac1c \uc791\uc5c5\uc774 \uc9c4\ud589 \ub610\ub294 \ub300\uae30 \uc911\uc785\ub2c8\ub2e4. \uc911\ub2e8\ud558\uace0 \uc885\ub8cc\ud560\uae4c\uc694?\n\uc774\ubbf8 \uc800\uc7a5\ud55c \ud30c\uc77c\uacfc \uc6d0\ubcf8\uc740 \uc720\uc9c0\ub429\ub2c8\ub2e4.`:'ClipCatch\ub97c \uc885\ub8cc\ud560\uae4c\uc694?\n\uc800\uc7a5\ud558\uc9c0 \uc54a\uc740 \uacb0\uacfc\ub294 \uc885\ub8cc \uc804\uc5d0 \uc800\uc7a5\ud574 \uc8fc\uc138\uc694.';
   if(!confirm(text))return;closing=true;
   const result=await fetch('/api/stop',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({force:state.active>0}),signal:AbortSignal.timeout(4000)});
   if(!result.ok)throw Error('\uc885\ub8cc \uc694\uccad\uc5d0 \uc2e4\ud328\ud588\uc2b5\ub2c8\ub2e4. \uc791\uc5c5 \uc0c1\ud0dc\ub97c \ud655\uc778\ud558\uace0 \ub2e4\uc2dc \ub20c\ub7ec \uc8fc\uc138\uc694.');
   let stopped=false;
   for(let i=0;i<16;i++){await new Promise(resolve=>setTimeout(resolve,350));try{await fetch('/api/health',{cache:'no-store',signal:AbortSignal.timeout(1000)});}catch(_){stopped=true;break;}}
   if(!stopped)throw Error('\uc885\ub8cc \uc694\uccad\uc740 \uc804\ub2ec\ub410\uc9c0\ub9cc \uc544\uc9c1 \uc2e4\ud589 \uc911\uc785\ub2c8\ub2e4.');
   closed=true;connection?.close();connection=null;if(window.clipcatchPollTimer)clearInterval(window.clipcatchPollTimer);
   document.body.replaceChildren();const box=document.createElement('section');box.style.cssText='text-align:center;padding:100px 20px;color:#f0f4eb;font-family:Segoe UI,sans-serif';const h=document.createElement('h2'),p=document.createElement('p');h.textContent='ClipCatch\uac00 \uc885\ub8cc\ub418\uc5c8\uc2b5\ub2c8\ub2e4.';p.textContent='\ub2e4\uc2dc \uc0ac\uc6a9\ud558\ub824\uba74 ClipCatch \uc544\uc774\ucf58\uc744 \uc2e4\ud589\ud574 \uc8fc\uc138\uc694.';box.append(h,p);document.body.append(box);
  }catch(e){closing=false;msg.textContent=e.message||'\uc885\ub8cc\ub97c \ud655\uc778\ud558\uc9c0 \ubabb\ud588\uc2b5\ub2c8\ub2e4. \ub2e4\uc2dc \ub20c\ub7ec \uc8fc\uc138\uc694.';}finally{if(!closed)btn.disabled=false;}
 };
})();
