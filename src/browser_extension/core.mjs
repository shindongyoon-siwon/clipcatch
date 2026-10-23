export function redditPost(raw) {
  try {
    const u=new URL(raw);
    if(u.protocol!=='https:' || !['www.reddit.com','old.reddit.com','reddit.com'].includes(u.hostname) || u.port || u.username || u.password)return null;
    const m=u.pathname.match(/^\/(?:(?:r|user)\/[^/]+\/)?comments\/([a-z0-9]{5,12})(?:\/|$)/i);
    if(!m)return null;
    for(const k of [...u.searchParams.keys()])if(k.toLowerCase().startsWith('utm_'))u.searchParams.delete(k);
    u.hash='';return {id:m[1].toLowerCase(),url:u.href};
  }catch(_){return null;}
}
export function mediaParts(raw) {
  try {
    if(typeof raw!=='string'||raw.length>4096)return null;
    const u=new URL(raw);
    if(u.protocol!=='https:'||u.host!=='v.redd.it'||u.username||u.password||u.hash)return null;
    const m=u.pathname.match(/^\/([a-z0-9]{5,40})\/([A-Za-z0-9_-]+)\.(mpd|m3u8|mp4)$/);
    if(!m)return null;
    const kind=m[3]==='mpd'||m[3]==='m3u8'?'manifest':/audio/i.test(m[2])?'audio':'video';
    const quality=Number((m[2].match(/(?:DASH_|AUDIO_)(\d+)/i)||[])[1])||0;
    return {id:m[1],kind,ext:m[3],quality,url:u.href};
  }catch(_){return null;}
}
export function groupMedia(domURLs, networkURLs, matched=true) {
  const dom=new Set(domURLs.map(mediaParts).filter(Boolean).map(x=>x.id));
  const groups=new Map();
  for(const raw of [...domURLs,...networkURLs]) {
    const m=mediaParts(raw);if(!m || (dom.size&&!dom.has(m.id)))continue;
    if(!groups.has(m.id))groups.set(m.id,{id:m.id,matched:matched&&dom.has(m.id),items:[]});
    const g=groups.get(m.id);if(!g.items.some(x=>x.url===m.url))g.items.push(m);
  }
  return [...groups.values()].map(g=>{
    const manifest=g.items.filter(x=>x.kind==='manifest').sort((a,b)=>(a.ext==='mpd'?0:1)-(b.ext==='mpd'?0:1))[0];
    const video=g.items.filter(x=>x.kind==='video').sort((a,b)=>b.quality-a.quality)[0];
    const audio=g.items.filter(x=>x.kind==='audio').sort((a,b)=>b.quality-a.quality)[0];
    return {id:g.id,matched:g.matched,media_url:manifest?.url||video?.url||'',audio_url:manifest?'':audio?.url||'',kind:manifest?manifest.ext:audio?'paired':'video-only',quality:video?.quality||0};
  }).filter(g=>g.media_url).slice(0,12);
}
export function pairingCode(raw) {
  const value=String(raw||'').trim();
  const u=new URL(value);
  if(u.protocol!=='http:'||!['127.0.0.1','localhost'].includes(u.hostname)||u.username||u.password||u.search||u.pathname!=='/'||!/^#[a-f0-9]{64}$/.test(u.hash))throw Error('Invalid ClipCatch pairing code');
  if(!u.port||Number(u.port)<1024||Number(u.port)>65535)throw Error('Invalid ClipCatch port');
  return {base:u.origin,code:u.hash.slice(1)};
}

// A browser tab can offer one explicitly selected public video-post URL.
// The downloader (not the extension) fetches the media. No cookie or request
// interception occurs outside the optional Reddit-only fallback.
function domainIs(host, root) { return host === root || host.endsWith("." + root); }
export function videoPage(raw) {
  try {
    if (typeof raw !== "string" || raw.length > 4096) return null;
    const u=new URL(raw);
    if(u.protocol!=="https:" || u.username || u.password || u.port || u.pathname.length>1500)return null;
    u.hash="";
    for(const k of [...u.searchParams.keys()]) {
      if (/^utm_/i.test(k) || /^(fbclid|gclid|igshid|si|feature|app|ref_src|ref_url)$/i.test(k))u.searchParams.delete(k);
    }
    const host=u.hostname.toLowerCase(),path=u.pathname;
    let platform="",type="page";
    if(["reddit.com","www.reddit.com","old.reddit.com","new.reddit.com"].includes(host)) {
      if (/^\/(?:(?:r|user)\/[^/]+\/)?comments\/[a-z0-9]{5,12}(?:\/|$)/i.test(path)) {platform="Reddit";type="reddit";}
    } else if (host==="redd.it"||host==="www.redd.it") {
      if(/^\/[a-z0-9]{5,12}\/?$/i.test(path)){platform="Reddit";type="reddit";}
    } else if (domainIs(host,"youtube.com")) {
      if((path==="/watch"&&/^[A-Za-z0-9_-]{11}$/.test(u.searchParams.get("v")||""))||/^\/(?:shorts|live)\/[A-Za-z0-9_-]{11}(?:\/|$)/.test(path))platform="YouTube";
    } else if (host==="youtu.be"||host==="www.youtu.be") {
      if(/^\/[A-Za-z0-9_-]{11}\/?$/.test(path))platform="YouTube";
    } else if (domainIs(host,"x.com")||domainIs(host,"twitter.com")) {
      if(/^\/(?:[^/]+\/status|i\/status)\/\d+(?:\/|$)/i.test(path))platform="X (트위터)";
    } else if (domainIs(host,"instagram.com")) {
      if(/^\/(?:p|reel|tv)\/[A-Za-z0-9_-]+(?:\/|$)/i.test(path))platform="Instagram";
    } else if (domainIs(host,"tiktok.com")) {
      if(/^\/@[^/]+\/video\/\d+(?:\/|$)/i.test(path)||/^\/t\/\w+\/?$/.test(path))platform="TikTok";
    } else if (domainIs(host,"facebook.com")||host==="fb.watch") {
      if((host==="fb.watch"&&/^\/[A-Za-z0-9_-]+\/?$/.test(path))||/^\/(?:reel|watch|videos|video\.php)(?:\/|$)/i.test(path)||/\/videos\/\d+/.test(path))platform="Facebook";
    } else if (domainIs(host,"vimeo.com")) {
      if(/^\/\d+(?:\/|$)/.test(path)||/^\/channels\/[^/]+\/\d+/.test(path))platform="Vimeo";
    } else if(domainIs(host,"streamable.com")) {
      if(/^\/[A-Za-z0-9]+\/?$/.test(path))platform="Streamable";
    } else if(domainIs(host,"twitch.tv")) {
      if(/^\/videos\/\d+(?:\/|$)/i.test(path)||/^\/[^/]+\/clip\/[^/]+/.test(path)||(host==="clips.twitch.tv"&&/^\/[^/]+/.test(path)))platform="Twitch";
    } else if(domainIs(host,"imgur.com")) {
      if(/^\/(?:gallery|a)\/[A-Za-z0-9]+(?:\/|$)/.test(path)||/^\/[A-Za-z0-9]+\/?$/.test(path))platform="Imgur";
    }
    return platform?{url:u.href,platform,type}:null;
  }catch(_){return null;}
}
