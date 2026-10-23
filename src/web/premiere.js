// Included in the executable; extends the existing downloader without a second app.
(() => {
  'use strict';
  const el=id=>document.getElementById(id);
  const style=document.createElement('style');
  style.textContent='.premiere-panel{margin:16px 0;padding:14px;border:1px solid #46523d;border-radius:12px;background:#1d261c}.premiere-controls{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.premiere-controls label{font-size:13px;font-weight:700}.premiere-controls select{max-width:100%;padding:9px;border-radius:7px;background:#171d25;color:#eef3e7;border:1px solid #526247;font:inherit}.premiere-controls button{padding:9px 13px;border:1px solid #697859;border-radius:8px;background:#2e3b26;color:#e8f3dc;font-weight:700;cursor:pointer}.premiere-controls button:disabled{opacity:.5;cursor:wait}.premiere-note{font-size:12px;line-height:1.55;margin:8px 0 0;color:#b8c7ab}.premiere-result{font-size:12px;line-height:1.5;margin-top:8px;white-space:pre-line;color:#d9ff57}.premiere-result.error{color:#ffb6bd}.premiere-convert{padding:8px 10px;border:1px solid #617252;border-radius:7px;background:#24331e;color:#d9ff57;font-size:12px;cursor:pointer}.premiere-controls :focus-visible,.premiere-convert:focus-visible{outline:2px solid #d9ff57;outline-offset:2px}';
  document.head.append(style);
  const panel=document.createElement('section');panel.className='premiere-panel';panel.id='premierePanel';
  const row=document.createElement('div');row.className='premiere-controls';
  const label=document.createElement('label');label.htmlFor='outputMode';label.textContent='저장 방식';
  const mode=document.createElement('select');mode.id='outputMode';
  for(const [value,text] of [['premiere','프리미어용 MP4 (권장)'],['original','원본 그대로']]){const option=document.createElement('option');option.value=value;option.textContent=text;mode.append(option);}
  const choose=document.createElement('button');choose.type='button';choose.id='convertLocalFiles';choose.textContent='기존 영상 MP4로 변환';
  const note=document.createElement('p');note.className='premiere-note';note.id='outputModeNote';
  const feedback=document.createElement('div');feedback.id='premiereFeedback';feedback.className='premiere-result';feedback.setAttribute('role','status');feedback.setAttribute('aria-live','polite');
  row.append(label,mode,choose);panel.append(row,note,feedback);
  const target=el('downloadBtn')?.closest('.action-row');if(target)target.before(panel);else document.querySelector('main')?.prepend(panel);
  let saving=false,choosing=false;
  const tell=(text,error=false)=>{feedback.textContent=text;feedback.classList.toggle('error',error);};
  const explain=()=>{
    const headline=document.querySelector('.hero h1 em');if(headline)headline.textContent=mode.value==='premiere'?'편집용 MP4로 저장.':'원본 그대로 저장.';
    note.textContent=mode.value==='premiere'?'같은 해상도의 H.264·AAC 우선. 필요한 부분만 변환하고 원본은 보존해.':'원본 스트림을 그대로 저장해. MKV·AV1·VP9는 프리미어에서 안 열릴 수 있어.';
    const support=document.querySelector('.action-row .support');if(support)support.textContent=mode.value==='premiere'?'편집용 MP4 · 해상도 유지 · 변환이 필요하면 시간이 더 걸려.':'최고 가용 화질 · 별도 재압축 없음 · MP4/MKV';
  };
  mode.addEventListener('change',async()=>{
    saving=true;mode.disabled=true;explain();
    try{await post('/api/premiere/settings',{mode:mode.value});tell('새로 추가하는 영상부터 적용돼.');}
    catch(e){tell(e.message,true);}finally{saving=false;mode.disabled=false;await poll();}
  });
  choose.addEventListener('click',async()=>{
    choosing=true;choose.disabled=true;tell('파일 선택창에서 영상을 골라 줘. 원본은 삭제하지 않아.');
    try{
      const r=await post('/api/premiere/choose',{});const items=r.items||[];const ok=items.filter(x=>!x.error);const bad=items.filter(x=>x.error);
      tell(items.length?ok.length+'개 변환 대기열에 추가했어. 결과는 아래 내역에 표시돼.'+(bad.length?'\n'+bad.map(x=>x.name+': '+x.error).join('\n'):''):'파일 선택을 취소했어.',bad.length>0);
    }catch(e){tell(e.message,true);}finally{choosing=false;choose.disabled=false;await poll();}
  });
  const oldJob=renderJob;
  renderJob=function(j){
    const card=oldJob(j);
    if(j.stage==='converting'){const status=card.querySelector('.job-status');if(status)status.textContent='MP4 변환 중';const cancel=card.querySelector('.job-cancel');if(cancel)cancel.textContent='변환 취소';}
    if(j.task==='convert' && !j.url){const link=card.querySelector('.job-url');if(link)link.textContent='PC에 저장된 영상 · 원본 보존';}
    if(j.task==='convert'||j.original_file){for(const btn of card.querySelectorAll('.retry-btn')){if(btn.textContent==='이 링크 재시도')btn.remove();}}
    if((j.file||j.original_file)&&!j.edit_ready&&['done','error','cancelled'].includes(j.status)){
      let actions=card.querySelector('.job-actions');if(!actions){actions=document.createElement('div');actions.className='job-actions';card.append(actions);}
      const btn=document.createElement('button');btn.className='premiere-convert';btn.type='button';btn.textContent='편집용 MP4 만들기';
      btn.addEventListener('click',async()=>{btn.disabled=true;try{await post('/api/premiere/convert',{id:j.id});tell('다시 다운로드하지 않고 MP4 사본을 만들게.');await poll();}catch(e){tell(e.message,true);btn.disabled=false;}});
      actions.append(btn);
    }
    return card;
  };
  const oldSignature=jobSignature;
  jobSignature=jobs=>oldSignature(jobs)+JSON.stringify(jobs.map(j=>[j.stage,j.original_file,j.edit_ready,j.task]));
  const oldRender=render;
  render=function(st){oldRender(st);if(st){if(!saving)mode.value=st.output_mode||'premiere';choose.disabled=choosing||st.phase!=='ready';explain();}};
  lastRecentHistorySignature='';lastOlderHistorySignature='';
  if(lastState)render(lastState);else explain();
})();
