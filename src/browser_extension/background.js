import {videoPage,redditPost,mediaParts,groupMedia,pairingCode,supportedSite,buildCandidates} from './core.mjs';
const TTL=10*60*1000;
let serial=Promise.resolve();
const serialTask=fn=>{const next=serial.then(fn,fn);serial=next.catch(()=>{});return next;};
async function currentTab(){const [tab]=await chrome.tabs.query({active:true,currentWindow:true});return tab;}
async function localRequest(link,path,body,authorized=true){
 const ctrl=new AbortController(),timer=setTimeout(()=>ctrl.abort(),12000);
 try{
  const base=new URL(link.base);
  if(base.protocol!=='http:'||!['127.0.0.1','localhost'].includes(base.hostname)||(!base.port||Number(base.port)<1024||Number(base.port)>65535)||base.username||base.password)throw Error('연결 대상이 ClipCatch가 아닙니다. ClipCatch에서 코드를 다시 복사해 주세요.');
  const r=await fetch(base.origin+path,{method:'POST',redirect:'error',credentials:'omit',headers:{'Content-Type':'application/json',...(authorized?{Authorization:'Bearer '+link.token}:{})},body:JSON.stringify(body),signal:ctrl.signal});
  let data;try{data=await r.json();}catch{throw Error('ClipCatch 프로그램을 업데이트해 주세요.');}
  if(!r.ok){const e=Error(data.error==='Invalid media request'?'ClipCatch 프로그램도 v1.7.2 이상으로 업데이트해 주세요.':data.error||'ClipCatch request failed');e.duplicate=!!data.duplicate||!!data.duplicates?.length;throw e;}return data;
 }catch(e){if(e.name==='AbortError'||e instanceof TypeError)throw Error('ClipCatch를 실행한 뒤 다시 시도해 주세요.');throw e;}finally{clearTimeout(timer);}
}
async function checkBackend(link){
 const r=await localRequest(link,'/bridge/status',{ids:[]});
 if(r.app!=='clipcatch'||r.channel!=='stable'||!r.capabilities?.includes('job-status'))throw Error('ClipCatch 전용 연결이 필요합니다. 운영 중인 ClipCatch를 실행해 주세요.');
 return r;
}
async function rememberJobs(result,title,mode,key=""){
 const ids=Array.isArray(result.job_ids)?result.job_ids:result.id?[result.id]:[];
 if(!ids.length)return;
 const {recentTransfers=[]}=await chrome.storage.local.get('recentTransfers');
 const created=Date.now();
 for(const id of ids){if(typeof id!=='string'||id.length>180)continue;if(!recentTransfers.some(x=>x.id===id))recentTransfers.unshift({id,key,title,output_mode:mode,created});}
 await chrome.storage.local.set({recentTransfers:recentTransfers.slice(0,30)});
}
async function discover(tab){
 if(!tab?.id||!supportedSite(tab.url))throw Error('유튜브·X·인스타·Reddit 영상 페이지를 열어 주세요.');
 let result;
 try{[{result}]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:['discover.js']});}catch{throw Error('페이지를 새로고침한 후 다시 열어 주세요.');}
 if(!result||result.page_url!==tab.url)throw Error('페이지가 변경되었습니다. 다시 찾아 주세요.');
 const after=await chrome.tabs.get(tab.id);if(after.url!==tab.url)throw Error('페이지가 변경되었습니다. 다시 찾아 주세요.');
 const state={tabId:tab.id,page_url:tab.url,started:Date.now(),candidates:buildCandidates(result),limited:!!result.limited};
 await chrome.storage.session.set({collection:state});return state;
}
function summary(c){const {key,title,platform,page_url,part,mode,kind,quality}=c;return {key,title,platform,page_url,part,mode,kind,quality};}
async function readOutputMode(value){
 const stored=(await chrome.storage.local.get('outputMode')).outputMode;
 const mode=value??stored??'premiere';
 if(!['original','premiere'].includes(mode))throw Error('저장 형식을 확인해 주세요.');
 return mode;
}
async function handle(m){
 if(m.kind==='hello')return {build:'1.9.2',channel:'stable',capabilities:['output-mode','job-status']};
 if(m.kind==='output-mode'){
  const mode=await readOutputMode(m.value);
  if(m.value!==undefined)await chrome.storage.local.set({outputMode:mode});
  return {mode};
 }
 const outputMode=['send-selected','download','send-page'].includes(m.kind)?await readOutputMode(m.output_mode):null;
 if(m.kind==='pair'){
  const code=pairingCode(m.value);
  if(['18765','18766'].includes(new URL(code.base).port))throw Error('테스트판 연결 주소는 사용할 수 없습니다. 운영판에서 연결 코드를 복사해 주세요.');
  const r=await localRequest(code,'/bridge/pair',{code:code.code,channel:'stable'},false);
  if(r.channel!=='stable')throw Error('ClipCatch 운영판이 아닌 프로그램입니다. 연결하지 않았습니다.');
  if(r.protocol!==1||!/^[a-f0-9]{64}$/.test(r.token))throw Error('Unsupported ClipCatch protocol');
  const link={base:code.base,token:r.token,version:r.version};await checkBackend(link);await chrome.storage.local.set({link,recentTransfers:[]});return {paired:true};
 }
 if(m.kind==='forget'){await chrome.storage.local.remove('link');await chrome.storage.local.remove('recentTransfers');return {paired:false};}
 const {link}=await chrome.storage.local.get('link');
 if(m.kind==='queue-status'){
  if(!link)return {paired:false,jobs:[]};
  const {recentTransfers=[]}=await chrome.storage.local.get('recentTransfers');
  const info=await localRequest(link,'/bridge/status',{ids:recentTransfers.map(x=>x.id).slice(0,30)});
  if(info.app!=='clipcatch'||info.channel!=='stable'||!info.capabilities?.includes('job-status'))throw Error('ClipCatch 연결을 확인해 주세요.');
  return {...info,paired:true,jobs:recentTransfers.map(x=>({...x,...(info.jobs.find(j=>j.id===x.id)||{status:'unavailable'})}))};
 }
 if(m.kind==='open-folder'){if(!link)throw Error('ClipCatch와 먼저 연결해 주세요.');await checkBackend(link);return localRequest(link,'/bridge/open-folder',{});}
 if(m.kind==='open-app'){if(!link)throw Error('먼저 ClipCatch와 연결해 주세요.');await checkBackend(link);await chrome.tabs.create({url:new URL(link.base).origin+'/'});return {ok:true};}
 if(['send-selected','download','send-page'].includes(m.kind)&&link){const state=await checkBackend(link);if(state.update_required||state.update_checking)throw Error('새 작업은 업데이트 확인·설치 후 시작할 수 있어요. [업데이트 안내 열기]를 눌러 주세요.');}
 const tab=await currentTab();
 if(m.kind==='list'){const c=await discover(tab);return {paired:!!link,platform:supportedSite(tab.url),candidates:c.candidates.map(summary),limited:c.limited};}
 if(m.kind==='send-selected'){
  if(link)await checkBackend(link);
  if(!link)throw Error('ClipCatch와 먼저 연결해 주세요.');
  if(!Array.isArray(m.keys)||!m.keys.length||m.keys.length>10||new Set(m.keys).size!==m.keys.length||m.keys.some(k=>typeof k!=='string'))throw Error('한 번에 1~10개를 선택해 주세요.');
  const {collection}=await chrome.storage.session.get('collection');
  if(!collection||Date.now()-collection.started>TTL||collection.tabId!==tab?.id||collection.page_url!==tab.url)throw Error('목록이 만료되었습니다. [다시 찾기]를 눌러 주세요.');
  if(m.keys.some(k=>!collection.candidates.some(c=>c.key===k)))throw Error('목록에 없는 영상입니다.');
  const fresh=await discover(tab),results=[];
  for(const key of m.keys){
   const c=fresh.candidates.find(c=>c.key===key);if(!c){results.push({key,ok:false,error:'화면의 영상이 변경되었습니다. 다시 선택해 주세요.'});continue;}
   try{
    const active=await currentTab();if(active?.id!==tab.id||active.url!==tab.url)throw Error('보고 있는 탭이 변경되었습니다. 나머지는 다시 선택해 주세요.');
    let r;
    if(c.mode==='media'){
     if(c.kind==='video-only'&&!m.silent)throw Error('음성을 찾지 못했습니다. 재생 후 다시 찾거나 원래 무음인 경우만 무음 저장을 선택해 주세요.');
     r=await localRequest(link,'/bridge/download',{output_mode:outputMode,page_url:c.page_url,title:c.title,media_url:c.media_url,audio_url:c.audio_url,allow_silent:!!m.silent,allow_duplicates:!!m.duplicates,separate:true});
    }else r=await localRequest(link,'/bridge/page',{output_mode:outputMode,page_url:c.page_url,allow_duplicates:!!m.duplicates});
    await rememberJobs(r,c.title,outputMode,key);results.push({key,ok:true,pending:!!r.already_queued||!!r.skipped_pending,job_ids:r.job_ids|| (r.id?[r.id]:[])});
   }catch(e){results.push({key,ok:false,error:e.message,duplicate:!!e.duplicate});}
  }return {results};
 }
 if(m.kind==='status'||m.kind==='scan'){
  const post=redditPost(tab?.url),page=videoPage(tab?.url);let {capture}=await chrome.storage.session.get('capture');
  if(capture&&(Date.now()-capture.started>TTL||capture.tabId!==tab?.id||capture.postId!==post?.id)){capture=null;await chrome.storage.session.remove('capture');}
  if(m.kind==='scan'){
   if(!post)throw Error('Reddit 개별 게시물을 열어 주세요. 목록에서는 위의 선택 다운로드를 사용해 주세요.');
   const [{result}]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:['capture.js']});
   if(!result||result.error||redditPost(result.page_url)?.id!==post.id)throw Error('게시물이 변경되었습니다.');
   capture={tabId:tab.id,postId:post.id,page_url:post.url,title:result.title,started:capture?.started||Date.now(),dom:result.dom_urls||[],matched:result.matched,network:[...new Set([...(capture?.network||[]),...(result.resource_urls||[])])].slice(-60)};
   await chrome.storage.session.set({capture});
  }
  let backend=null;try{if(link)backend=await checkBackend(link);}catch(e){backend={error:e.message};}
  return {paired:!!link&&!backend?.error,backend,page_url:page?.url||'',platform:page?.platform||'',title:tab?.title||'',page_type:post?'reddit':'page',capturing:!!capture,groups:capture?groupMedia(capture.dom,capture.network,capture.matched):[]};
 }
 if(m.kind==='download'){
  if(!link)throw Error('ClipCatch와 먼저 연결해 주세요.');const {capture}=await chrome.storage.session.get('capture'),post=redditPost(tab?.url);
  if(!capture||capture.tabId!==tab?.id||capture.postId!==post?.id||Date.now()-capture.started>TTL)throw Error('영상을 다시 찾아 주세요.');
  const g=groupMedia(capture.dom,capture.network,capture.matched).find(g=>g.id===m.id);if(!g||!g.matched&&!m.confirmed)throw Error('선택한 영상을 확인해 주세요.');
  await checkBackend(link);const result=await localRequest(link,'/bridge/download',{output_mode:outputMode,page_url:capture.page_url,title:capture.title,media_url:g.media_url,audio_url:g.audio_url,allow_silent:!!m.silent,allow_duplicates:!!m.duplicates,separate:true});await rememberJobs(result,capture.title,outputMode,"media:"+g.id);return result;
 }
 if(m.kind==='send-page'){const p=videoPage(tab?.url);if(!link||!p)throw Error('개별 영상 게시물을 선택해 주세요.');await checkBackend(link);const result=await localRequest(link,'/bridge/page',{output_mode:outputMode,page_url:p.url,allow_duplicates:!!m.duplicates});await rememberJobs(result,tab?.title||p.platform,outputMode);return result;}
 throw Error('Unknown request');
}
chrome.runtime.onMessage.addListener((m,sender,reply)=>{
 if(sender.id!==chrome.runtime.id||sender.url!==chrome.runtime.getURL('popup.html')){reply({ok:false,error:'Extension popup only'});return false;}
 serialTask(()=>handle(m)).then(data=>reply({ok:true,data}),e=>reply({ok:false,error:e.message,duplicate:!!e.duplicate}));return true;
});
chrome.webRequest.onCompleted.addListener(d=>{
 if(d.tabId<0||d.statusCode<200||d.statusCode>=300||!mediaParts(d.url))return;
 serialTask(async()=>{const {capture}=await chrome.storage.session.get('capture');if(!capture||capture.tabId!==d.tabId||Date.now()-capture.started>TTL)return;let tab;try{tab=await chrome.tabs.get(d.tabId);}catch{return;}if(redditPost(tab.url)?.id!==capture.postId){await chrome.storage.session.remove('capture');return;}capture.network=[...new Set([...(capture.network||[]),d.url])].slice(-60);await chrome.storage.session.set({capture});}).catch(()=>{});
},{urls:['https://v.redd.it/*']});
chrome.tabs.onRemoved.addListener(id=>serialTask(async()=>{const s=await chrome.storage.session.get(['capture','collection']);for(const k of ['capture','collection'])if(s[k]?.tabId===id)await chrome.storage.session.remove(k);}).catch(()=>{}));
