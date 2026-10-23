const $=id=>document.getElementById(id);
function notice(text,error=false){$('status').textContent=text;$('status').classList.toggle('error',error);}
async function message(data){
  const result=await chrome.runtime.sendMessage(data);
  if(!result?.ok){const e=Error(result?.error||'요청에 실패했어.');e.duplicate=!!result?.duplicate;throw e;}
  return result.data;
}
function render(state){
  $('connectionTitle').textContent=state.paired?'ClipCatch 연결됨 · 다시 연결하려면 열기':'처음 한 번 · ClipCatch 연결';
  if(!state.paired)$('connection').open=true;
  $('platform').textContent=state.platform||'지원하는 영상 게시물 페이지를 열어 줘';
  $('pageTitle').textContent=state.platform?state.title:'';
  $('pageURL').textContent=state.page_url||'';
  $('send').disabled=!state.paired||!state.page_url;
  $('pageHint').textContent=!state.paired?'먼저 위에서 ClipCatch 프로그램과 연결해 줘.':state.page_url?'영상은 ClipCatch가 다운로드해. 로그인 제한이나 접근 제한이 있으면 실패할 수 있어.':'영상 게시물 하나를 열어 줘. 사이트 홈이나 검색 화면은 안 돼.';
  $('redditTools').hidden=state.page_type!=='reddit';
  $('scanHint').textContent=state.capturing?'영상이 안 보이면 게시물에서 재생하고 다시 찾아 줘.':'Reddit에서 영상을 재생한 뒤 눌러 줘.';
  $('candidates').replaceChildren();
  for(const g of state.groups||[]){
    const box=document.createElement('div');box.className='candidate';
    const label=document.createElement('b');label.textContent=g.matched?'현재 게시물에서 확인한 영상':'현재 탭에서 감지한 영상 후보';
    const info=document.createElement('small');
    info.textContent=(g.kind==='mpd'||g.kind==='m3u8'?'영상·음성 목록':g.kind==='paired'?'영상 + 음성':'영상만 · 음성 확인 필요')+(g.quality?' · '+g.quality+'p':'')+'\nID: '+g.id;
    const btn=document.createElement('button');btn.textContent='이 영상 ClipCatch로 보내기';btn.disabled=!state.paired;
    btn.onclick=async()=>{
      const confirmed=g.matched||confirm('게시물과 영상이 일치하는지 확인했어?');
      if(!confirmed)return;
      btn.disabled=true;notice('영상 전송 중…');
      const data={kind:'download',id:g.id,silent:$('silent').checked,confirmed};
      try{
        let outcome;
        try{outcome=await message(data);}
        catch(e){if(!e.duplicate||!confirm(e.message))throw e;outcome=await message({...data,duplicates:true});}
        notice(outcome.already_queued?'이미 다운로드 대기 중이야.':'ClipCatch로 보냈어. 저장 결과는 프로그램에서 확인해 줘.');
      }catch(e){notice(e.message,true);}finally{btn.disabled=false;}
    };
    box.append(label,info,btn);$('candidates').append(box);
  }
}
$('pair').onclick=async()=>{
  $('pair').disabled=true;
  try{await message({kind:'pair',value:$('pairCode').value});$('pairCode').value='';$('connection').open=false;render(await message({kind:'status'}));notice('연결 완료! 이제 영상 게시물에서 다운로드할 수 있어.');}
  catch(e){notice(e.message,true);}finally{$('pair').disabled=false;}
};
$('send').onclick=async()=>{
  $('send').disabled=true;notice('ClipCatch에 보내는 중…');
  try{
    let outcome;
    try{outcome=await message({kind:'send-page'});}
    catch(e){if(!e.duplicate||!confirm(e.message))throw e;outcome=await message({kind:'send-page',duplicates:true});}
    notice(outcome.count>0?'다운로드 대기열에 추가했어. ClipCatch에서 완료를 확인해 줘.':outcome.skipped_pending>0?'이미 다운로드 대기 중인 영상이야.':'추가할 새로운 영상이 없어.');
  }catch(e){notice(e.message,true);}
  finally{const state=await message({kind:'status'}).catch(()=>null);if(state)render(state);}
};
$('scan').onclick=async()=>{
  $('scan').disabled=true;
  try{const state=await message({kind:'scan'});render(state);notice(state.groups.length?'영상 후보를 찾았어. 저장할 영상을 선택해 줘.':'아직 영상 주소를 못 찾았어. 영상을 재생한 다음 다시 눌러 줘.');}
  catch(e){notice(e.message,true);}finally{$('scan').disabled=false;}
};
message({kind:'status'}).then(render).catch(e=>notice(e.message,true));
