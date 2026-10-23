import {videoPage,redditPost,mediaParts,groupMedia,pairingCode,supportedSite,buildCandidates} from './core.mjs';
const TTL=10*60*1000;
let serial=Promise.resolve();
const serialTask=fn=>{const next=serial.then(fn,fn);serial=next.catch(()=>{});return next;};
async function currentTab(){const [tab]=await chrome.tabs.query({active:true,currentWindow:true});return tab;}
async function localRequest(link,path,body,authorized=true){
 const ctrl=new AbortController(),timer=setTimeout(()=>ctrl.abort(),12000);
 try{
  const base=new URL(link.base);
  if(base.protocol!=='http:'||!['127.0.0.1','localhost'].includes(base.hostname)||!base.port||base.username||base.password)throw Error('Invalid local connection');
  const r=await fetch(base.origin+path,{method:'POST',credentials:'omit',headers:{'Content-Type':'application/json',...(authorized?{Authorization:'Bearer '+link.token}:{})},body:JSON.stringify(body),signal:ctrl.signal});
  let data;try{data=await r.json();}catch{throw Error('ClipCatch 프로그램을 업데이트해 줘.');}
  if(!r.ok){const e=Error(data.error==='Invalid media request'?'ClipCatch 프로그램도 v1.7.2 이상으로 업데이트해 줘.':data.error||'ClipCatch request failed');e.duplicate=!!data.duplicate||!!data.duplicates?.length;throw e;}return data;
 }catch(e){if(e.name==='AbortError'||e instanceof TypeError)throw Error('ClipCatch를 실행한 뒤 다시 시도해 줘.');throw e;}finally{clearTimeout(timer);}
}
async function discover(tab){
 if(!tab?.id||!supportedSite(tab.url))throw Error('유튜브·X·인스타·Reddit 등 영상이 있는 페이지를 열어 줘.');
 let result;
 try{[{result}]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:['discover.js']});}catch{throw Error('페이지를 새로고침한 뒤 확장프로그램을 다시 열어 줘.');}
 if(!result||result.page_url!==tab.url)throw Error('페이지가 바뀌었어. 다시 찾아 줘.');
 const after=await chrome.tabs.get(tab.id);if(after.url!==tab.url)throw Error('페이지가 바뀌었어. 다시 찾아 줘.');
 const state={tabId:tab.id,page_url:tab.url,started:Date.now(),candidates:buildCandidates(result),limited:!!result.limited};
 await chrome.storage.session.set({collection:state});return state;
}
function summary(c){const {key,title,platform,page_url,part,mode,kind,quality}=c;return {key,title,platform,page_url,part,mode,kind,quality};}
async function handle(m){
 if(m.kind==='pair'){
  const code=pairingCode(m.value),r=await localRequest(code,'/bridge/pair',{code:code.code},false);
  if(r.protocol!==1||!/^[a-f0-9]{64}$/.test(r.token))throw Error('Unsupported ClipCatch protocol');
  await chrome.storage.local.set({link:{base:code.base,token:r.token,version:r.version}});return {paired:true};
 }
 if(m.kind==='forget'){await chrome.storage.local.remove('link');return {paired:false};}
 const {link}=await chrome.storage.local.get('link'),tab=await currentTab();
 if(m.kind==='list'){const c=await discover(tab);return {paired:!!link,platform:supportedSite(tab.url),candidates:c.candidates.map(summary),limited:c.limited};}
 if(m.kind==='send-selected'){
  if(!link)throw Error('ClipCatch와 먼저 연결해 줘.');
  if(!Array.isArray(m.keys)||!m.keys.length||m.keys.length>10||new Set(m.keys).size!==m.keys.length||m.keys.some(k=>typeof k!=='string'))throw Error('한 번에 1~10개를 선택해 줘.');
  const {collection}=await chrome.storage.session.get('collection');
  if(!collection||Date.now()-collection.started>TTL||collection.tabId!==tab?.id||collection.page_url!==tab.url)throw Error('목록을 다시 읽어 줘. 페이지가 바뀌었거나 시간이 지났어.');
  if(m.keys.some(k=>!collection.candidates.some(c=>c.key===k)))throw Error('목록에 없는 영상이야.');
  const fresh=await discover(tab),results=[];
  for(const key of m.keys){
   const c=fresh.candidates.find(c=>c.key===key);if(!c){results.push({key,ok:false,error:'화면에서 영상이 바뀌었어. 다시 선택해 줘.'});continue;}
   try{
    const active=await currentTab();if(active?.id!==tab.id||active.url!==tab.url)throw Error('보고 있는 탭이 바뀌었어. 나머지는 다시 선택해 줘.');
    let r;
    if(c.mode==='media'){
     if(c.kind==='video-only'&&!m.silent)throw Error('음성을 찾지 못했어. 재생 후 다시 찾거나 원래 무음인 경우만 무음 저장을 선택해 줘.');
     r=await localRequest(link,'/bridge/download',{page_url:c.page_url,title:c.title,media_url:c.media_url,audio_url:c.audio_url,allow_silent:!!m.silent,allow_duplicates:!!m.duplicates,separate:true});
    }else r=await localRequest(link,'/bridge/page',{page_url:c.page_url,allow_duplicates:!!m.duplicates});
    results.push({key,ok:true,pending:!!r.already_queued||!!r.skipped_pending});
   }catch(e){results.push({key,ok:false,error:e.message,duplicate:!!e.duplicate});}
  }return {results};
 }
 if(m.kind==='status'||m.kind==='scan'){
  const post=redditPost(tab?.url),page=videoPage(tab?.url);let {capture}=await chrome.storage.session.get('capture');
  if(capture&&(Date.now()-capture.started>TTL||capture.tabId!==tab?.id||capture.postId!==post?.id)){capture=null;await chrome.storage.session.remove('capture');}
  if(m.kind==='scan'){
   if(!post)throw Error('Reddit 개별 게시물을 열어 줘. 목록에서는 위의 선택 다운로드를 사용해 줘.');
   const [{result}]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:['capture.js']});
   if(!result||result.error||redditPost(result.page_url)?.id!==post.id)throw Error('게시물이 바뀌었어.');
   capture={tabId:tab.id,postId:post.id,page_url:post.url,title:result.title,started:capture?.started||Date.now(),dom:result.dom_urls||[],matched:result.matched,network:[...new Set([...(capture?.network||[]),...(result.resource_urls||[])])].slice(-60)};
   await chrome.storage.session.set({capture});
  }
  return {paired:!!link,page_url:page?.url||'',platform:page?.platform||'',title:tab?.title||'',page_type:post?'reddit':'page',capturing:!!capture,groups:capture?groupMedia(capture.dom,capture.network,capture.matched):[]};
 }
 if(m.kind==='download'){
  if(!link)throw Error('ClipCatch와 먼저 연결해 줘.');const {capture}=await chrome.storage.session.get('capture'),post=redditPost(tab?.url);
  if(!capture||capture.tabId!==tab?.id||capture.postId!==post?.id||Date.now()-capture.started>TTL)throw Error('영상을 다시 찾아 줘.');
  const g=groupMedia(capture.dom,capture.network,capture.matched).find(g=>g.id===m.id);if(!g||!g.matched&&!m.confirmed)throw Error('선택한 영상을 확인해 줘.');
  return localRequest(link,'/bridge/download',{page_url:capture.page_url,title:capture.title,media_url:g.media_url,audio_url:g.audio_url,allow_silent:!!m.silent,allow_duplicates:!!m.duplicates,separate:true});
 }
 if(m.kind==='send-page'){const p=videoPage(tab?.url);if(!link||!p)throw Error('개별 영상 게시물을 선택해 줘.');return localRequest(link,'/bridge/page',{page_url:p.url,allow_duplicates:!!m.duplicates});}
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
