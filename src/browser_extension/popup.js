import {transferSummary,candidateJobs} from './transfer-state.mjs';
const $=id=>document.getElementById(id);
let candidates=[],selected=new Set(),outcomes=new Map(),paired=false,working=false,duplicates=[],polling=false;
let protocolReady=false,updateBlocked=false;
let latestJobs=[],batchIDs=[],batchFailures=0,hasBatch=false;const itemNodes=new Map();
function notice(t,error=false){$('status').textContent=t;$('status').classList.toggle('error',error);}
async function message(m){const r=await chrome.runtime.sendMessage(m);if(!r?.ok){const raw=r?.error||'Request failed';const mismatch=raw==='Unknown request'||String(raw).includes('CC-EXT-VERSION');if(mismatch)$('reloadExtension').hidden=false;const e=Error(mismatch?'확장프로그램 화면과 실행부 버전이 다릅니다. [확장 새로고침]을 누른 뒤 다시 열어 주세요.':raw);e.duplicate=!!r?.duplicate;throw e;}return r.data;}
function buttons(){
 $('send').disabled=working||!paired||updateBlocked||!selected.size;
 $('updateApp').hidden=!updateBlocked;
 $('scan').disabled=working||updateBlocked;
 $('send').textContent=updateBlocked?'업데이트 후 사용 가능':working?'보내는 중...':selected.size?`선택한 ${selected.size}개 항목 다운로드`:'영상을 선택해 주세요';
 $('selectionCount').textContent=`${selected.size}개 선택`;
 $('selectAll').checked=candidates.length>0&&selected.size===Math.min(10,candidates.length);
 $('outputMode').disabled=working;$('openDownloads').disabled=!paired;
 $('selectAll').disabled=working;$('refresh').disabled=working;
}
function render(){
 itemNodes.clear();$('count').textContent=`선택 항목 ${candidates.length}개`;$('list').replaceChildren();$('empty').hidden=!!candidates.length;
 $('silentRow').hidden=!candidates.some(c=>c.kind==='video-only');
 for(const [i,c] of candidates.entries()){
  const row=document.createElement('div');row.className='video-item';const check=document.createElement('input');check.type='checkbox';check.checked=selected.has(c.key);check.disabled=working;check.setAttribute('aria-label',`${i+1}. ${c.title}`);
  check.onchange=()=>{if(check.checked){if(selected.size>=10){check.checked=false;notice('한 번에 최대 10개까지 선택해 주세요.');return;}selected.add(c.key);}else selected.delete(c.key);buttons();};
  const box=document.createElement('div'),badge=document.createElement('small');badge.className='video-meta';badge.textContent=c.platform+(c.part?' · 게시물 안 영상 '+c.part:c.mode==='page'?' · 게시물 영상 전체':'');
  const title=document.createElement('strong');title.textContent=c.title||c.platform;const source=document.createElement('small');source.className='video-url';source.textContent=new URL(c.page_url).pathname;
  const result=document.createElement('small');result.className='item-status';
  const folder=document.createElement('button');folder.type='button';folder.className='folder-link';folder.textContent=U.folder;folder.hidden=true;folder.onclick=openDownloads;
  itemNodes.set(c.key,{result,folder});

  box.append(badge,title,source,result,folder);row.append(check,box);$('list').append(row);
 }updateInlineStatuses();buttons();
}
function modeNote(){ $('modeNote').textContent=$('outputMode').value==='premiere'?'H.264·AAC MP4로 저장합니다. 변환은 PC에서 처리됩니다.':'편집용 재변환 없이 저장합니다. MP4·MKV 등 원본 형식에 따라 달라집니다.'; }
$('outputMode').onchange=async()=>{try{await message({kind:'output-mode',value:$('outputMode').value});modeNote();}catch(e){notice(e.message,true);}};
async function load(){
 protocolReady=false;working=true;buttons();
 try{
  const hello=await message({kind:'hello'});
  if(hello?.build!=='1.9.2'||hello?.channel!=='stable'){ $('reloadExtension').hidden=false;throw Error('CC-EXT-VERSION: 확장 새로고침 후 다시 열어 주세요.');}
  protocolReady=true;$('reloadExtension').hidden=true;
  const preference=await message({kind:'output-mode'});$('outputMode').value=preference.mode;modeNote();
  const state=await message({kind:'status'});paired=state.paired;updateBlocked=!!state.backend?.update_required||!!state.backend?.update_checking;$('connectionTitle').textContent=paired?'ClipCatch 연결됨':'처음 한 번 · ClipCatch 연결';if(!paired)$('connection').open=true;$('redditTools').hidden=state.page_type!=='reddit';
  const data=await message({kind:'list'});candidates=data.candidates;selected=new Set([...selected].filter(k=>candidates.some(c=>c.key===k)));if(candidates.length===1)selected.add(candidates[0].key);
  $('connectionBadge').textContent=state.backend?.error|| (paired?'ClipCatch '+state.backend?.version+' \u00b7 \uc5f0\uacb0\ub428':'ClipCatch\uc640 \uc5f0\uacb0\ud574 \uc8fc\uc138\uc694.');
  notice(data.limited?'불러온 항목 중 최대 40개를 표시합니다.':paired?'':'위에서 ClipCatch와 연결해 주세요.');
 }catch(e){candidates=[];selected.clear();notice(e.message,true);}finally{working=false;render();refreshJobs();}
}
async function send(keys,allowDuplicates=false){
 if(!keys.length)return;working=true;batchIDs=[];batchFailures=0;hasBatch=true;duplicates=[];$('retryDuplicates').hidden=true;render();notice('선택한 영상을 ClipCatch로 전송하고 있습니다.');
 try{
  const r=await message({kind:'send-selected',keys,output_mode:$('outputMode').value,silent:$('silent').checked,duplicates:allowDuplicates});let sent=0,failed=0;
  for(const x of r.results){outcomes.set(x.key,x);batchIDs.push(...(x.job_ids||[]));if(x.ok){selected.delete(x.key);sent++;}else{failed++;if(x.duplicate)duplicates.push(x.key);}}
  batchIDs=[...new Set(batchIDs)];batchFailures=failed;
  $('retryDuplicates').hidden=!duplicates.length;notice(sent?U.preparing+(failed?' \u00b7 '+failed+U.needCheck:''):U.failed,failed>0);
 }catch(e){notice(e.message,true);}finally{working=false;render();refreshJobs();}
}
$('send').onclick=()=>send([...selected]);
$('retryDuplicates').onclick=()=>{if(confirm('이미 받은 영상을 사본으로 다시 받으시겠습니까?'))send([...duplicates],true);};
$('refresh').onclick=load;$('selectAll').onchange=()=>{selected=$('selectAll').checked?new Set(candidates.slice(0,10).map(c=>c.key)):new Set();render();};
$('pair').onclick=async()=>{try{await message({kind:'pair',value:$('pairCode').value});$('pairCode').value='';$('connection').open=false;await load();}catch(e){notice(e.message,true);}};
$('scan').onclick=async()=>{
 try{
  const r=await message({kind:'scan'});$('candidates').replaceChildren();
  for(const [i,g] of r.groups.entries()){
   const b=document.createElement('button');b.textContent=`감지된 영상 ${i+1} 저장`;
   b.onclick=async()=>{try{if(!g.matched&&!confirm('선택한 영상이 원하는 게시물의 영상이 맞습니까?'))return;await message({kind:'download',id:g.id,output_mode:$('outputMode').value,confirmed:true,silent:$('silent').checked});notice('ClipCatch로 전송했습니다.');}catch(e){notice(e.message,true);}};
   $('candidates').append(b);
  }if(!r.groups.length)notice('게시물에서 영상을 재생한 후 다시 찾아 주세요.');
 }catch(e){notice(e.message,true);}
};
const T={"done": "\ub2e4\uc6b4\ub85c\ub4dc \uc644\ub8cc", "files": "\uac1c \ud30c\uc77c", "failed": "\uc2e4\ud328", "cancelled": "\ucde8\uc18c\ub428", "unavailable": "\uae30\ub85d \uc5c6\uc74c", "queued": "\ub2e4\uc6b4\ub85c\ub4dc \ub300\uae30 \uc911", "verifying": "\ud30c\uc77c \uac80\uc0ac \uc911", "converting": "MP4 \ubcc0\ud658 \uc911", "downloading": "\ub2e4\uc6b4\ub85c\ub4dc \uc911", "connected": "\uc5f0\uacb0\ub428", "video": "\uc601\uc0c1", "original": "\uc6d0\ubcf8", "sep": " \u00b7 "};
function stageLabel(j){
 if(j.status==='done')return Number(j.file_count)>0?T.done+T.sep+j.file_count+T.files:U.verifying;
 if(j.status==='error')return T.failed+(j.error_code?' ('+j.error_code+')':'');
 if(j.status==='cancelled')return T.cancelled;
 if(j.status==='unavailable')return T.unavailable;
 if(j.status==='queued')return T.queued;
 if(j.stage==='verifying')return T.verifying;
 if(j.stage==='converting'||j.stage==='encoding')return T.converting;
 return T.downloading;
}
async function refreshJobs(){
 if(polling||working||!protocolReady)return;polling=true;
 try{
  const r=await message({kind:'queue-status'});
  if(!r.paired){paired=false;buttons();return;}
  paired=true;updateBlocked=!!r.update_required||!!r.update_checking;$('connectionBadge').textContent='ClipCatch '+r.version+T.sep+T.connected;
  latestJobs=r.jobs||[];updateInlineStatuses();updateBatchStatus();
  const list=$('transfers');list.replaceChildren();$('transferSection').hidden=!r.jobs.length;
  for(const j of r.jobs){
   const row=document.createElement('div');row.className='transfer '+j.status;
   const title=document.createElement('strong');title.textContent=j.title||T.video;
   const text=document.createElement('p');text.textContent=(j.output_mode==='premiere'?'MP4':T.original)+T.sep+stageLabel(j)+(j.status==='downloading'?' '+Math.round(j.percent||0)+'%':'');
   row.append(title,text);
   if(['queued','downloading'].includes(j.status)){const bar=document.createElement('progress');bar.max=100;bar.value=j.percent||0;row.append(bar);}
   if(j.status==='done'&&Number(j.file_count)>0){const folder=document.createElement('button');folder.type='button';folder.className='folder-link';folder.textContent=U.folder;folder.onclick=openDownloads;row.append(folder);}
   list.append(row);
  }
  buttons();
 }catch(e){paired=false;$('connectionBadge').textContent=e.message;buttons();}finally{polling=false;}
}
$('openDownloads').onclick=openDownloads;
$('updateApp').onclick=async()=>{try{await message({kind:'open-app'});}catch(e){notice(e.message,true);}};
$('reloadExtension').onclick=()=>chrome.runtime.reload();

const U={"folder": "\uc800\uc7a5 \ud3f4\ub354 \uc5f4\uae30", "preparing": "\ub2e4\uc6b4\ub85c\ub4dc \uc900\ube44 \uc911", "done": "\ub2e4\uc6b4\ub85c\ub4dc \uc644\ub8cc", "failed": "\ub2e4\uc6b4\ub85c\ub4dc \uc2e4\ud328", "needCheck": "\uac1c \ud655\uc778 \ud544\uc694", "files": "\uac1c \ud30c\uc77c \uc800\uc7a5", "unknown": "\uc644\ub8cc \uc5ec\ubd80 \ud655\uc778 \ud544\uc694", "verifying": "\uc800\uc7a5 \uacb0\uacfc \ud655\uc778 \uc911", "pending": "\ub2e4\uc6b4\ub85c\ub4dc \uc9c4\ud589 \ud655\uc778 \uc911", "progress": "\ucc98\ub9ac \uc911", "partial": "\uc77c\ubd80 \uc800\uc7a5 \uc644\ub8cc", "folderFail": "\uc800\uc7a5 \ud3f4\ub354\ub97c \uc5f4\uc9c0 \ubabb\ud588\uc2b5\ub2c8\ub2e4."};
async function openDownloads(){
 try{const r=await message({kind:'open-folder'});if(!r?.ok)throw Error(U.folderFail);}
 catch(e){notice(e.message,true);}
}
function updateInlineStatuses(){
 for(const c of candidates){
  const nodes=itemNodes.get(c.key);if(!nodes)continue;
  const o=outcomes.get(c.key),jobs=candidateJobs(c.key,o,latestJobs);
  const sum=transferSummary(jobs,o?.job_ids?.length||jobs.length);
  let text='',error=false;
  if(o&&!o.ok){text=o.error;error=true;}
  else if(sum.state==='done'){text=U.done+' \u00b7 '+sum.files+U.files;}
  else if(jobs.length){
   if(sum.failed){text=(sum.files?U.partial+' \u00b7 '+sum.files+U.files:U.failed)+' / '+sum.failed+U.needCheck;error=true;}
   else if(jobs.length===1){text=stageLabel(jobs[0])+(jobs[0].status==='downloading'?' '+Math.round(jobs[0].percent||0)+'%':'');}
   else{text=U.progress+' \u00b7 '+sum.done+'/'+sum.total;}
  }else if(o?.ok){text=U.pending;}
  nodes.result.textContent=text;nodes.result.classList.toggle('error',error);nodes.result.classList.toggle('done',sum.state==='done');nodes.folder.hidden=sum.files<=0;
 }
}
function updateBatchStatus(){
 if(!hasBatch||working)return;
 const jobs=batchIDs.map(id=>latestJobs.find(j=>j.id===id)).filter(Boolean);
 const sum=transferSummary(jobs,batchIDs.length);
 if(sum.state==='done'&&!batchFailures)notice(U.done+' \u00b7 '+sum.files+U.files);
 else if(sum.state==='active')notice((jobs.find(j=>j.status==='downloading')?stageLabel(jobs.find(j=>j.status==='downloading')):U.preparing)+(sum.done?' \u00b7 '+sum.done+'/'+sum.total:''));
 else if(sum.failed||batchFailures)notice((sum.files?U.partial+' \u00b7 '+sum.files+U.files:U.failed)+' / '+(sum.failed+batchFailures)+U.needCheck,true);
 else if(sum.state==='unknown')notice(U.unknown,true);
 else notice(batchIDs.length?U.preparing:U.pending);
}

setInterval(()=>{if(document.visibilityState==='visible')refreshJobs();},1200);
load();
