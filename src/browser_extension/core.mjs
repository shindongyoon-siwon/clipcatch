// Only selected public video posts and matched Reddit media enter the downloader.
function domainIs(host,root){return host===root||host.endsWith('.'+root);}
function baseURL(raw){try{if(typeof raw!=='string'||raw.length>4096)return null;const u=new URL(raw);if(u.protocol!=='https:'||u.username||u.password||u.port||u.pathname.length>1500)return null;u.hash='';for(const k of [...u.searchParams.keys()])if(/^utm_/i.test(k)||/^(fbclid|gclid|igshid|si|feature|app|ref_src|ref_url)$/i.test(k))u.searchParams.delete(k);return u;}catch{return null;}}
export function videoPage(raw){
 const u=baseURL(raw);if(!u)return null;const host=u.hostname,path=u.pathname;let platform='',type='page';
 if(['reddit.com','www.reddit.com','old.reddit.com','new.reddit.com'].includes(host)){if(/^\/(?:(?:r|user)\/[^/]+\/)?comments\/[a-z0-9]{5,12}(?:\/|$)/i.test(path)){platform='Reddit';type='reddit';}}
 else if(host==='redd.it'||host==='www.redd.it'){if(/^\/[a-z0-9]{5,12}\/?$/i.test(path)){platform='Reddit';type='reddit';}}
 else if(domainIs(host,'youtube.com')){if(path==='/watch'&&/^[A-Za-z0-9_-]{11}$/.test(u.searchParams.get('v')||'')||/^\/(shorts|live)\/[A-Za-z0-9_-]{11}(?:\/|$)/.test(path))platform='YouTube';}
 else if(host==='youtu.be'||host==='www.youtu.be'){if(/^\/[A-Za-z0-9_-]{11}\/?$/.test(path))platform='YouTube';}
 else if(domainIs(host,'x.com')||domainIs(host,'twitter.com')){if(/^\/(?:[^/]+\/status|i\/status)\/\d+(?:\/|$)/i.test(path))platform='X (트위터)';}
 else if(domainIs(host,'instagram.com')){if(/^\/(p|reel|tv)\/[A-Za-z0-9_-]+(?:\/|$)/i.test(path))platform='Instagram';}
 else if(domainIs(host,'tiktok.com')){if(/^\/@[^/]+\/video\/\d+(?:\/|$)/i.test(path)||/^\/t\/\w+\/?$/.test(path))platform='TikTok';}
 else if(domainIs(host,'facebook.com')||host==='fb.watch'){if(host==='fb.watch'&&/^\/[A-Za-z0-9_-]+\/?$/.test(path)||/^\/(reel|watch|videos|video\.php)(\/|$)/i.test(path)||/\/videos\/\d+/.test(path))platform='Facebook';}
 else if(domainIs(host,'vimeo.com')){if(/^\/\d+(?:\/|$)/.test(path)||/^\/channels\/[^/]+\/\d+/.test(path))platform='Vimeo';}
 else if(domainIs(host,'streamable.com')){if(/^\/[A-Za-z0-9]+\/?$/.test(path))platform='Streamable';}
 else if(domainIs(host,'twitch.tv')){if(/^\/videos\/\d+(?:\/|$)/i.test(path)||/^\/[^/]+\/clip\/[^/]+/.test(path)||host==='clips.twitch.tv'&&/^\/[^/]+/.test(path))platform='Twitch';}
 else if(domainIs(host,'imgur.com')){if(/^\/(?:gallery|a)\/[A-Za-z0-9]+(?:\/|$)/.test(path)||/^\/[A-Za-z0-9]+\/?$/.test(path))platform='Imgur';}
 return platform?{url:u.href,platform,type}:null;
}
export function redditPost(raw){const p=videoPage(raw);if(!p||p.type!=='reddit')return null;const u=new URL(p.url);if(['redd.it','www.redd.it'].includes(u.hostname)){const id=u.pathname.replaceAll('/','');return {id:id.toLowerCase(),url:'https://www.reddit.com/comments/'+id+'/'};}const m=u.pathname.match(/^\/(?:(?:r|user)\/[^/]+\/)?comments\/([a-z0-9]{5,12})(?:\/|$)/i);return m?{id:m[1].toLowerCase(),url:u.href}:null;}
export function mediaParts(raw){try{if(typeof raw!=='string'||raw.length>4096)return null;const u=new URL(raw);if(u.protocol!=='https:'||u.host!=='v.redd.it'||u.username||u.password||u.hash)return null;const m=u.pathname.match(/^\/([a-z0-9]{5,40})\/([A-Za-z0-9_-]+)\.(mpd|m3u8|mp4)$/);if(!m)return null;const kind=m[3]==='mpd'||m[3]==='m3u8'?'manifest':/audio/i.test(m[2])?'audio':'video';return {id:m[1],kind,ext:m[3],quality:Number((m[2].match(/(?:DASH_|AUDIO_)(\d+)/i)||[])[1])||0,url:u.href};}catch{return null;}}
export function groupMedia(domURLs,networkURLs,matched=true){
 const dom=new Set(domURLs.map(mediaParts).filter(Boolean).map(x=>x.id)),groups=new Map();
 for(const raw of [...domURLs,...networkURLs]){const m=mediaParts(raw);if(!m||dom.size&&!dom.has(m.id))continue;if(!groups.has(m.id))groups.set(m.id,{id:m.id,matched:matched&&dom.has(m.id),items:[]});const g=groups.get(m.id);if(!g.items.some(x=>x.url===m.url))g.items.push(m);}
 return [...groups.values()].map(g=>{const manifest=g.items.filter(x=>x.kind==='manifest').sort((a,b)=>(a.ext==='mpd'?0:1)-(b.ext==='mpd'?0:1))[0],video=g.items.filter(x=>x.kind==='video').sort((a,b)=>b.quality-a.quality)[0],audio=g.items.filter(x=>x.kind==='audio').sort((a,b)=>b.quality-a.quality)[0];return {id:g.id,matched:g.matched,media_url:manifest?.url||video?.url||'',audio_url:manifest?'':audio?.url||'',kind:manifest?manifest.ext:audio?'paired':'video-only',quality:video?.quality||0};}).filter(g=>g.media_url).slice(0,12);
}
export function pairingCode(raw){const u=new URL(String(raw||'').trim());if(u.protocol!=='http:'||!['127.0.0.1','localhost'].includes(u.hostname)||u.username||u.password||u.search||u.pathname!=='/'||!/^#[a-f0-9]{64}$/.test(u.hash)||!u.port||Number(u.port)<1024||Number(u.port)>65535)throw Error('Invalid ClipCatch pairing code');return {base:u.origin,code:u.hash.slice(1)};}
export function supportedSite(raw){const u=baseURL(raw);if(!u)return '';const sites=[['reddit.com','Reddit'],['redd.it','Reddit'],['youtube.com','YouTube'],['youtu.be','YouTube'],['x.com','X'],['twitter.com','X'],['instagram.com','Instagram'],['facebook.com','Facebook'],['fb.watch','Facebook'],['tiktok.com','TikTok'],['vimeo.com','Vimeo'],['streamable.com','Streamable'],['twitch.tv','Twitch'],['imgur.com','Imgur']];return sites.find(([d])=>domainIs(u.hostname,d))?.[1]||'';}
export function pageKey(raw){const p=videoPage(raw);if(!p)return '';const u=new URL(p.url),r=redditPost(p.url);if(r)return 'reddit:'+r.id;if(p.platform==='YouTube')return 'youtube:'+(u.searchParams.get('v')||u.pathname.split('/').filter(Boolean).at(-1));if(p.platform==='X (트위터)'){const m=u.pathname.match(/\/status\/(\d+)(?:\/video\/(\d+))?/);return m?'x:'+m[1]+(m[2]?':video:'+m[2]:''):p.url;}return u.hostname.replace(/^www\./,'')+u.pathname.replace(/\/$/,'');}
export function wholePostPage(raw){
 const p=videoPage(raw);if(!p)return null;const u=new URL(p.url);
 if(p.platform==='X (\ud2b8\uc704\ud130)')u.pathname=u.pathname.replace(/(\/status\/\d+)\/video\/\d+\/?$/, '$1');
 if(p.platform==='Instagram')u.searchParams.delete('img_index');
 return {...p,url:u.href};
}
export function buildCandidates(d){
 if(!d||!supportedSite(d.page_url))return [];const found=[],seen=new Set(),site=supportedSite(d.page_url);
 function push(c){if(found.length<40&&!seen.has(c.key)){found.push(c);seen.add(c.key);}}
 for(const r of (d.records||[]).slice(0,60)){const p=wholePostPage(r.page_url);if(!p||supportedSite(p.url)!==site)continue;const base=pageKey(p.url),title=String(r.title||p.platform).slice(0,250),groups=p.type==='reddit'&&r.matched===true?groupMedia((r.dom_urls||[]).slice(0,60),[],true):[];
  if(groups.length)groups.forEach((g,i)=>push({key:base+':media:'+g.id,page_url:p.url,platform:p.platform,title,mode:'media',media_id:g.id,media_url:g.media_url,audio_url:g.audio_url,kind:g.kind,quality:g.quality,part:groups.length>1?`${i+1}/${groups.length}`:'',matched:true}));
  else push({key:base,page_url:p.url,platform:p.platform,title,mode:'page',part:''});
 }
 const current=wholePostPage(d.page_url);if(current){const k=pageKey(current.url);if(!found.some(c=>pageKey(c.page_url)===k))push({key:k,page_url:current.url,platform:current.platform,title:String(d.title||current.platform).slice(0,250),mode:'page',part:''});found.sort((a,b)=>Number(pageKey(b.page_url)===k)-Number(pageKey(a.page_url)===k));}
 return found;
}
