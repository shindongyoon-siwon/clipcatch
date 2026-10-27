(() => {
 'use strict';
 const button=document.getElementById('folderBtn');if(!button)return;
 const feedback=document.createElement('p');feedback.id='folderFeedback';feedback.hidden=true;feedback.setAttribute('role','status');feedback.style.cssText='font-size:12px;line-height:1.6;overflow-wrap:anywhere;margin:8px 0;color:#aab7c6';button.parentElement.after(feedback);
 button.addEventListener('click',async()=>{
  if(button.disabled)return;button.disabled=true;feedback.hidden=false;feedback.textContent='폴더 열기 요청 중…';
  let path=typeof lastState!=='undefined'&&lastState?lastState.save_dir||'':'';
  try{
   const response=await fetch('/api/open-folder',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}',signal:AbortSignal.timeout(8000)});
   const data=await response.json();if(typeof data.path==='string')path=data.path;
   if(!response.ok||data.ok!==true||data.requested!==true)throw Error(data.error||'폴더 열기 요청에 실패했어.');
   feedback.textContent='탐색기에 폴더 열기를 요청했어. 창이 보이지 않으면 작업표시줄의 탐색기를 확인해 줘.';
  }catch(error){
   const detail=error.name==='TimeoutError'||error.name==='AbortError'?'폴더 열기 응답을 확인하지 못했어.':error.message||'폴더를 열지 못했어.';
   feedback.textContent=detail+(path?' 폴더 경로: '+path:'');
  }finally{button.disabled=false;}
 });
})();
