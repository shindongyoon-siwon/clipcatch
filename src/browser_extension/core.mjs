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
