// Included in the executable; extends the existing downloader without a second app.
(() => {
  'use strict';
  const el=id=>document.getElementById(id);
  const style=document.createElement('style');
  style.textContent='.premiere-panel{margin:16px 0;padding:14px;border:1px solid #46523d;border-radius:12px;background:#1d261c}.premiere-controls{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.premiere-controls label{font-size:13px;font-weight:700}.premiere-controls select{max-width:100%;padding:9px;border-radius:7px;background:#171d25;color:#eef3e7;border:1px solid #526247;font:inherit}.premiere-controls button{padding:9px 13px;border:1px solid #697859;border-radius:8px;background:#2e3b26;color:#e8f3dc;font-weight:700;cursor:pointer}.premiere-controls button:disabled{opacity:.5;cursor:wait}.premiere-note{font-size:12px;line-height:1.55;margin:8px 0 0;color:#b8c7ab}.premiere-result{font-size:12px;line-height:1.5;margin-top:8px;white-space:pre-line;color:#d9ff57}.premiere-result.error{color:#ffb6bd}.premiere-convert{padding:8px 10px;border:1px solid #617252;border-radius:7px;background:#24331e;color:#d9ff57;font-size:12px;cursor:pointer}.premiere-controls :focus-visible,.premiere-convert:focus-visible{outline:2px solid #d9ff57;outline-offset:2px}';
  style.textContent+=`.mp4-help{position:relative;display:inline-block;margin:0;padding:0;background:none}.mp4-help summary{display:flex;align-items:center;justify-content:center;list-style:none;cursor:pointer;border:1px solid #798e70;border-radius:50%;width:24px;height:24px;font-weight:700;font-size:13px}.mp4-help summary::-webkit-details-marker{display:none}.mp4-help-copy{position:absolute;top:30px;right:0;width:280px;max-width:75vw;background:#152017;color:#e4eadd;border:1px solid #63775b;border-radius:8px;padding:13px;white-space:pre-line;font-size:12px;line-height:1.7;z-index:30;box-shadow:0 4px 20px #0008}`;
  document.head.append(style);
  style.textContent+='.premiere-panel{margin:18px 0 0;padding:0;border:0;background:transparent}.premiere-controls select{background:#141b24;border-color:#43515f}.premiere-note{color:#93a3b4}.local-conversion{margin-top:10px;font-size:12px;color:#a8b9cb}.local-conversion>summary{cursor:pointer;width:fit-content;padding:4px 0}.local-conversion .premiere-controls{margin-top:9px}.post-files{margin-top:12px;padding-top:10px;border-top:1px solid #374352;color:#afbfce;font-size:12px}.post-files summary{cursor:pointer;line-height:1.6}.post-file{overflow-wrap:anywhere;margin:8px 0;line-height:1.65}.post-file.error{color:#ffb8c1}';
  const panel=document.createElement('section');panel.className='premiere-panel';panel.id='premierePanel';
  const row=document.createElement('div');row.className='premiere-controls';
  const label=document.createElement('label');label.htmlFor='outputMode';label.textContent='저장 방식';
  const mode=document.createElement('select');mode.id='outputMode';
  for(const [value,text] of [['premiere','프리미어용 MP4 (권장)'],['original','원본 그대로']]){const option=document.createElement('option');option.value=value;option.textContent=text;mode.append(option);}
  const choose=document.createElement('button');choose.type='button';choose.id='convertLocalFiles';choose.textContent='기존 영상 MP4로 변환';
  const note=document.createElement('p');note.className='premiere-note';note.id='outputModeNote';
  const feedback=document.createElement('div');feedback.id='premiereFeedback';feedback.className='premiere-result';feedback.setAttribute('role','status');feedback.setAttribute('aria-live','polite');
  const help=document.createElement('details');help.className='mp4-help';help.id='mp4Help';
  const helpButton=document.createElement('summary');helpButton.textContent='?';helpButton.setAttribute('aria-label',"\ubcc0\ud658 \uc804 \ud655\uc778");helpButton.title="\ubcc0\ud658 \uc2dc \ud654\uc9c8\u00b7\uc6a9\ub7c9 \uc548\ub0b4";
  const helpCopy=document.createElement('div');helpCopy.className='mp4-help-copy';helpCopy.textContent="\uc601\uc0c1\u00b7\uc74c\uc131\uc744 \uadf8\ub300\ub85c \ub2f4\uc73c\uba74 \uc7ac\uc555\ucd95\uc5d0 \ub530\ub978 \uc190\uc2e4\uc774 \uc5c6\uc2b5\ub2c8\ub2e4.\n\ud615\uc2dd\uc744 \ubc14\uafb8\uae30 \uc704\ud574 \ub2e4\uc2dc \uc555\ucd95\ud558\uba74 \ud654\uc9c8\u00b7\uc74c\uc9c8\uc774 \ub2ec\ub77c\uc9c8 \uc218 \uc788\uace0, \ucc98\ub9ac \uc2dc\uac04\uacfc \ud30c\uc77c \uc6a9\ub7c9\uc774 \ub298\uc5b4\ub0a0 \uc218 \uc788\uc2b5\ub2c8\ub2e4.\n\uc6d0\ubcf8\uc740 \ubcf4\uc874\ub429\ub2c8\ub2e4. \ud638\ud658\uc131 \uc7ac\ubcc0\ud658\uc740 \uc0c1\uc2dc \uc7ac\uc555\ucd95\ud558\uba70 \ud504\ub808\uc784\uc744 \uc870\uc815\ud560 \uc218 \uc788\uc2b5\ub2c8\ub2e4.";
  help.append(helpButton,helpCopy);row.append(label,mode,help);
  const localTools=document.createElement('details');localTools.className='local-conversion';localTools.id='localConversionTools';
  const localSummary=document.createElement('summary');localSummary.textContent='기존 파일 변환';
  const localActions=document.createElement('div');localActions.className='premiere-controls';localActions.append(choose);
  localTools.append(localSummary,localActions);panel.append(row,note,localTools,feedback);

  const repair=document.createElement('button');repair.type='button';repair.id='repairPremiere';repair.textContent='프리미어 호환성 재변환';repair.title='Premiere Pro에서 열리지 않거나 재생이 불안정할 때 사용합니다. 원본을 보존하고 호환성 우선 MP4 사본을 생성합니다.';localActions.append(repair);
  repair.onclick=async()=>{
    if(!confirm('호환성 우선 MP4 사본을 생성하시겠습니까? H.264/AAC·고정 프레임로 재압축하며 화질과 프레임이 달라질 수 있습니다. 원본은 보존됩니다.'))return;
    repair.disabled=true;try{const r=await post('/api/premiere/choose',{safe:true});tell((r.items||[]).filter(x=>!x.error).length+'개 호환성 재변환 대기');await poll();}catch(e){tell(e.message,true);}finally{repair.disabled=false;}
  };
  const target=el('downloadBtn')?.closest('.action-row');if(target)target.before(panel);else document.querySelector('main')?.prepend(panel);
  let saving=false,choosing=false;
  const tell=(text,error=false)=>{feedback.textContent=text;feedback.classList.toggle('error',error);};
  const explain=()=>{
    note.textContent=mode.value==='premiere'?'편집용 MP4로 저장합니다. 필요한 경우 변환 시간이 추가됩니다.':'재압축 없이 저장합니다. 원본 형식에 따라 편집 프로그램에서 열리지 않을 수 있습니다.';
  };
  mode.addEventListener('change',async()=>{
    saving=true;mode.disabled=true;explain();
    try{await post('/api/premiere/settings',{mode:mode.value});tell('새로 추가하는 다운로드부터 적용됩니다.');}
    catch(e){tell(e.message,true);}finally{saving=false;mode.disabled=false;await poll();}
  });
  choose.addEventListener('click',async()=>{
    choosing=true;choose.disabled=true;tell('변환할 영상을 선택해 주세요. 원본은 보존됩니다.');
    try{
      const r=await post('/api/premiere/choose',{});const items=r.items||[];const ok=items.filter(x=>!x.error);const bad=items.filter(x=>x.error);
      tell(items.length?ok.length+'개를 변환 대기열에 추가했습니다. 결과는 아래 내역에 표시됩니다.'+(bad.length?'\n'+bad.map(x=>x.name+': '+x.error).join('\n'):''):'파일 선택을 취소했습니다.',bad.length>0);
    }catch(e){tell(e.message,true);}finally{choosing=false;choose.disabled=false;await poll();}
  });
  const oldJob=renderJob;
  renderJob=function(j){
    const card=oldJob(j);
    if(j.stage==='converting'){const status=card.querySelector('.job-status');if(status)status.textContent='MP4 변환 중';const cancel=card.querySelector('.job-cancel');if(cancel)cancel.textContent='변환 취소';}
    if(j.task==='convert' && !j.url){const link=card.querySelector('.job-url');if(link)link.textContent='PC에 저장된 영상 · 원본 보존';}
    if(j.task==='convert'||j.original_file){for(const btn of card.querySelectorAll('.retry-btn')){if(btn.textContent==='이 링크 재시도')btn.remove();}}
    if(j.task!=='quality'&&j.task!=='quality-engine'&&(j.file||j.original_file)&&!j.edit_ready&&['done','error','cancelled'].includes(j.status)){
      let actions=card.querySelector('.job-actions');if(!actions){actions=document.createElement('div');actions.className='job-actions';card.append(actions);}
      const btn=document.createElement('button');btn.className='premiere-convert';btn.type='button';btn.textContent='편집용 MP4 만들기';
      btn.addEventListener('click',async()=>{btn.disabled=true;try{const result=await post('/api/premiere/convert',{id:j.id});if(result.items){const failed=result.items.filter(x=>x.error);tell((result.items.length-failed.length)+'개 변환 대기열에 추가'+(failed.length?' · '+failed.length+'개 확인 필요':''),!!failed.length);}else tell('다시 다운로드하지 않고 MP4 사본을 생성합니다.');await poll();}catch(e){tell(e.message,true);btn.disabled=false;}});
      actions.append(btn);
    }
    if(j.media?.length && (j.media.length>1 || j.media.some(m=>m.error))){
      const failed=j.media.filter(m=>m.error).length;
      const box=document.createElement('details');box.className='post-files';
      const summary=document.createElement('summary');summary.textContent=failed?'파일별 결과 · '+failed+'개 확인 필요':'저장된 영상 '+j.media.length+'개 보기';box.append(summary);box.open=failed>0;
      for(const [i,m] of j.media.entries()){
        const line=document.createElement('div');line.className=m.error?'post-file error':'post-file';
        const name=(m.file||m.original||'').split(/[\\/]/).pop()||'영상 '+(i+1);
        line.textContent=name+' · '+(m.error?(m.error_code?m.error_code+' · ':'')+m.error:m.edit_ready?'MP4 준비 완료':'원본 저장');
        box.append(line);
      }
      card.append(box);
    }
    return card;
  };
  const oldSignature=jobSignature;
  jobSignature=jobs=>oldSignature(jobs)+JSON.stringify(jobs.map(j=>[j.stage,j.original_file,j.edit_ready,j.task,j.media,j.download_incomplete]));
  const oldRender=render;
  render=function(st){oldRender(st);if(st){if(!saving)mode.value=st.output_mode||'premiere';choose.disabled=choosing||st.phase!=='ready';explain();}};
  lastRecentHistorySignature='';lastOlderHistorySignature='';
  if(lastState)render(lastState);else explain();
})();
