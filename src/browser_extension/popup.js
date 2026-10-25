const $=id=>document.getElementById(id);
let candidates=[],selected=new Set(),outcomes=new Map(),paired=false,working=false,duplicates=[];
function notice(t,error=false){$('status').textContent=t;$('status').classList.toggle('error',error);}
async function message(m){const r=await chrome.runtime.sendMessage(m);if(!r?.ok){const e=Error(r?.error||'Request failed');e.duplicate=!!r?.duplicate;throw e;}return r.data;}
function buttons(){
 $('send').disabled=working||!paired||!selected.size;
 $('send').textContent=working?'보내는 중...':selected.size?`선택한 ${selected.size}개 항목 다운로드`:'영상을 선택해 줘';
 $('selectionCount').textContent=`${selected.size}개 선택`;
 $('selectAll').checked=candidates.length>0&&selected.size===Math.min(10,candidates.length);
 $('selectAll').disabled=working;$('refresh').disabled=working;
}
function render(){
 $('count').textContent=`선택 항목 ${candidates.length}개`;$('list').replaceChildren();$('empty').hidden=!!candidates.length;
 $('silentRow').hidden=!candidates.some(c=>c.kind==='video-only');
 for(const [i,c] of candidates.entries()){
  const row=document.createElement('label');row.className='video-item';const check=document.createElement('input');check.type='checkbox';check.checked=selected.has(c.key);check.disabled=working;check.setAttribute('aria-label',`${i+1}. ${c.title}`);
  check.onchange=()=>{if(check.checked){if(selected.size>=10){check.checked=false;notice('한 번에 최대 10개까지 선택해 줘.');return;}selected.add(c.key);}else selected.delete(c.key);buttons();};
  const box=document.createElement('div'),badge=document.createElement('small');badge.className='video-meta';badge.textContent=c.platform+(c.part?' \u00b7 \uac8c\uc2dc\ubb3c \uc548 \uc601\uc0c1 '+c.part:c.mode==='page'?' \u00b7 \uac8c\uc2dc\ubb3c \uc601\uc0c1 \uc804\uccb4':'');
  const title=document.createElement('strong');title.textContent=c.title||c.platform;const source=document.createElement('small');source.className='video-url';source.textContent=new URL(c.page_url).pathname;
  const result=document.createElement('small');result.className='item-status';const o=outcomes.get(c.key);if(o){result.textContent=o.ok?(o.pending?'이미 대기 중':'다운로드 대기열에 넣었어'):o.error;result.classList.toggle('error',!o.ok);}
  box.append(badge,title,source,result);row.append(check,box);$('list').append(row);
 }buttons();
}
async function load(){
 working=true;buttons();
 try{
  const state=await message({kind:'status'});paired=state.paired;$('connectionTitle').textContent=paired?'ClipCatch 연결됨':'처음 한 번 · ClipCatch 연결';if(!paired)$('connection').open=true;$('redditTools').hidden=state.page_type!=='reddit';
  const data=await message({kind:'list'});candidates=data.candidates;selected=new Set([...selected].filter(k=>candidates.some(c=>c.key===k)));if(candidates.length===1)selected.add(candidates[0].key);
  notice(data.limited?'먼저 불러온 40개를 표시해.':paired?'':'위에서 ClipCatch와 연결해 줘.');
 }catch(e){candidates=[];selected.clear();notice(e.message,true);}finally{working=false;render();}
}
async function send(keys,allowDuplicates=false){
 if(!keys.length)return;working=true;duplicates=[];$('retryDuplicates').hidden=true;render();notice('선택한 영상을 ClipCatch로 보내고 있어.');
 try{
  const r=await message({kind:'send-selected',keys,silent:$('silent').checked,duplicates:allowDuplicates});let sent=0,failed=0;
  for(const x of r.results){outcomes.set(x.key,x);if(x.ok){selected.delete(x.key);sent++;}else{failed++;if(x.duplicate)duplicates.push(x.key);}}
  $('retryDuplicates').hidden=!duplicates.length;notice(`${sent}개 대기열 전송${failed?' · '+failed+'개 확인 필요':''}. 저장 완료는 ClipCatch에서 확인해 줘.`,failed>0);
 }catch(e){notice(e.message,true);}finally{working=false;render();}
}
$('send').onclick=()=>send([...selected]);
$('retryDuplicates').onclick=()=>{if(confirm('이미 받은 영상을 별도 사본으로 다시 받을까?'))send([...duplicates],true);};
$('refresh').onclick=load;$('selectAll').onchange=()=>{selected=$('selectAll').checked?new Set(candidates.slice(0,10).map(c=>c.key)):new Set();render();};
$('pair').onclick=async()=>{try{await message({kind:'pair',value:$('pairCode').value});$('pairCode').value='';$('connection').open=false;await load();}catch(e){notice(e.message,true);}};
$('scan').onclick=async()=>{
 try{
  const r=await message({kind:'scan'});$('candidates').replaceChildren();
  for(const [i,g] of r.groups.entries()){
   const b=document.createElement('button');b.textContent=`감지된 영상 ${i+1} 저장`;
   b.onclick=async()=>{try{if(!g.matched&&!confirm('이 영상이 원하는 게시물의 영상인지 확인했어?'))return;await message({kind:'download',id:g.id,confirmed:true,silent:$('silent').checked});notice('ClipCatch로 보냈어.');}catch(e){notice(e.message,true);}};
   $('candidates').append(b);
  }if(!r.groups.length)notice('게시물에서 영상을 재생한 후 다시 찾아 줘.');
 }catch(e){notice(e.message,true);}
};
load();
