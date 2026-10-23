import {videoPage,redditPost,mediaParts,groupMedia,pairingCode} from './core.mjs';
const TTL=10*60*1000;
let serial=Promise.resolve();
const serialTask=fn=>{const next=serial.then(fn,fn);serial=next.catch(()=>{});return next;};
async function currentPage() {
  const [tab]=await chrome.tabs.query({active:true,currentWindow:true});
  return {tab,page:videoPage(tab?.url)};
}
async function activePost() {
  const [tab]=await chrome.tabs.query({active:true,currentWindow:true});
  const post=redditPost(tab?.url);if(!tab||!post)throw Error('\uc800\uc7a5\ud560 Reddit \uc6d0\ubcf8 \uac8c\uc2dc\ubb3c \ud0ed\uc744 \uba3c\uc800 \uc5f4\uc5b4 \uc918.');
  return {tab,post};
}
async function scan(tab,post,old={}) {
  const [{result}]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:['capture.js']});
  if(!result||result.error||redditPost(result.page_url)?.id!==post.id)throw Error('\uac8c\uc2dc\ubb3c\uc774 \ubc14\ub00c\uc5c8\uc5b4. \ub2e4\uc2dc \ucc3e\uc544 \uc918.');
  const captured={tabId:tab.id,postId:post.id,page_url:post.url,title:result.title,started:old.started||Date.now(),dom:result.dom_urls||[],matched:result.matched,network:[...new Set([...(old.network||[]),...(result.resource_urls||[])])].slice(-60)};
  await chrome.storage.session.set({capture:captured});return captured;
}
async function localRequest(link,path,body,authorized=true) {
  const controller=new AbortController();const timer=setTimeout(()=>controller.abort(),10000);
  try {
    const response=await fetch(link.base+path,{method:'POST',credentials:'omit',headers:{'Content-Type':'application/json',...(authorized?{Authorization:'Bearer '+link.token}:{})},body:JSON.stringify(body),signal:controller.signal});
    let result;try{result=await response.json();}catch(_){throw Error('ClipCatch 1.7.0 \uc774\uc0c1\uc73c\ub85c \uc5c5\ub370\uc774\ud2b8\ud574 \uc918.');}
    if(!response.ok){const e=Error(result.error||'ClipCatch request failed');e.duplicate=!!result.duplicate || (Array.isArray(result.duplicates)&&result.duplicates.length>0);throw e;}return result;
  }catch(e){if(e.name==='AbortError'||e instanceof TypeError)throw Error('ClipCatch\ub97c \uc2e4\ud589\ud574 \uc918. \uacc4\uc18d \uc5f0\uacb0\uc774 \uc548 \ub418\uba74 \uc0c8 \uc5f0\uacb0 \ucf54\ub4dc\ub85c \ub2e4\uc2dc \uc5f0\uacb0\ud574 \uc918.');throw e;}finally{clearTimeout(timer);}
}
async function handle(message) {
  if(message.kind==='pair'){
    let pair;try{pair=pairingCode(message.value);}catch(_){throw Error('ClipCatch\uc758 [\uc5f0\uacb0 \ucf54\ub4dc \ub9cc\ub4e4\uae30]\uc5d0\uc11c \ubcf5\uc0ac\ud55c \uc804\uccb4 \ucf54\ub4dc\ub97c \ubd99\uc5ec\ub123\uc5b4 \uc918.');}
    const r=await localRequest(pair,'/bridge/pair',{code:pair.code},false);
    if(r.protocol!==1||!/^[a-f0-9]{64}$/.test(r.token))throw Error('Unsupported ClipCatch protocol');
    await chrome.storage.local.set({link:{base:pair.base,token:r.token,version:r.version}});return {paired:true};
  }
  if(message.kind==='forget'){await chrome.storage.local.remove('link');return {paired:false};}
  if(message.kind==='status'||message.kind==='scan') {
    const {link}=await chrome.storage.local.get('link');
    let {capture}=await chrome.storage.session.get('capture');
    const {tab,page}=await currentPage();
    const post=redditPost(tab?.url);
    if(capture && (Date.now()-capture.started>TTL || capture.tabId!==tab?.id || capture.postId!==post?.id)){
      capture=null;await chrome.storage.session.remove('capture');
    }
    if(message.kind==='scan'){
      if(!tab||!post)throw Error('먼저 Reddit 원본 게시물을 열어 줘.');
      capture=await scan(tab,post,capture||{});
    }
    return {paired:!!link,version:link?.version,page_url:page?.url||'',platform:page?.platform||'',page_type:post?'reddit':'page',title:capture?.title||tab?.title||'',capturing:!!capture,groups:capture?groupMedia(capture.dom,capture.network,capture.matched):[]};
  }
  if(message.kind==='send-page'){
    const {link}=await chrome.storage.local.get('link');
    if(!link)throw Error('ClipCatch와 먼저 연결해 줘.');
    const {page}=await currentPage();
    if(!page)throw Error('영상이 있는 게시물 한 개를 열어 줘. 사이트 홈이나 검색 화면은 안 돼.');
    const result=await localRequest(link,'/bridge/page',{page_url:page.url,allow_duplicates:!!message.duplicates});
    return {...result,platform:page.platform};
  }
  if(message.kind==='download') {
    const {link}=await chrome.storage.local.get('link');if(!link)throw Error('ClipCatch\uc640 \uba3c\uc800 \uc5f0\uacb0\ud574 \uc918.');
    const {capture}=await chrome.storage.session.get('capture');const {tab,post}=await activePost();
    if(!capture||Date.now()-capture.started>TTL||capture.tabId!==tab.id||capture.postId!==post.id)throw Error('\uc601\uc0c1\uc744 \ub2e4\uc2dc \ucc3e\uc544 \uc918.');
    const group=groupMedia(capture.dom,capture.network,capture.matched).find(g=>g.id===message.id);
    if(!group)throw Error('\uc601\uc0c1 \uc8fc\uc18c\ub97c \ucc3e\uc9c0 \ubabb\ud588\uc5b4.');
    if(!group.matched&&!message.confirmed)throw Error('\uac8c\uc2dc\ubb3c\uacfc \uc601\uc0c1\uc758 \ub300\uc751\uc744 \ud655\uc778\ud574 \uc918.');
    return localRequest(link,'/bridge/download',{page_url:capture.page_url,title:capture.title,media_url:group.media_url,audio_url:group.audio_url,allow_silent:!!message.silent,allow_duplicates:!!message.duplicates});
  }
  throw Error('Unknown request');
}
chrome.runtime.onMessage.addListener((message,sender,reply)=>{
  if(sender.id!==chrome.runtime.id || sender.url!==chrome.runtime.getURL('popup.html')){reply({ok:false,error:'Extension popup only'});return false;}
  serialTask(()=>handle(message)).then(data=>reply({ok:true,data}),e=>reply({ok:false,error:e.message,duplicate:!!e.duplicate}));return true;
});
// Only requests made by the explicitly armed tab are kept. No headers or
// cookies are read, and no other tabs or sites are logged.
chrome.webRequest.onCompleted.addListener(details=>{
  if(details.tabId<0||!mediaParts(details.url)||details.statusCode<200||details.statusCode>=300)return;
  serialTask(async()=>{
    const {capture}=await chrome.storage.session.get('capture');
    if(!capture||capture.tabId!==details.tabId||Date.now()-capture.started>TTL)return;
    let tab;try{tab=await chrome.tabs.get(details.tabId);}catch(_){return;}
    if(redditPost(tab.url)?.id!==capture.postId){await chrome.storage.session.remove('capture');return;}
    capture.network=[...new Set([...(capture.network||[]),details.url])].slice(-60);
    await chrome.storage.session.set({capture});
  }).catch(()=>{});
},{urls:['https://v.redd.it/*']});
chrome.tabs.onRemoved.addListener(id=>serialTask(async()=>{const {capture}=await chrome.storage.session.get('capture');if(capture?.tabId===id)await chrome.storage.session.remove('capture');}).catch(()=>{}));
