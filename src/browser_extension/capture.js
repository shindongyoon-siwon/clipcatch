// On-demand DOM read only. No page functions, cookies, storage, request headers,
// challenge solving, or browser login databases are accessed.
(() => {
  const post=(location.pathname.match(/^\/(?:(?:r|user)\/[^/]+\/)?comments\/([a-z0-9]{5,12})(?:\/|$)/i)||[])[1];
  if(!post)return {error:'Open a Reddit post first'};
  const posts=[...document.querySelectorAll('shreddit-post')];
  const root=posts.find(el=>el.id==='t3_'+post || el.getAttribute('thingid')==='t3_'+post || (el.getAttribute('permalink')||'').includes('/comments/'+post+'/')) || document.querySelector('[data-fullname="t3_'+post+'"]');
  const area=root||(!posts.length?(document.querySelector('main')||document.body):null);
  const urls=new Set();let visited=0;
  function read(value,depth=0) {
    if(depth>8 || urls.size>=48)return;
    if(typeof value==='string') {
      const s=value.replace(/\\\//g,'/').replace(/&amp;/g,'&');
      for(const m of s.matchAll(/https:\/\/v\.redd\.it\/[a-z0-9]+\/[A-Za-z0-9_-]+\.(?:mpd|m3u8|mp4)(?:\?[^\s"'<>\\]*)?/g))urls.add(m[0]);
      if(s.length<200000 && /^[\[{]/.test(s.trim()))try{read(JSON.parse(s),depth+1);}catch(_){}
    }else if(Array.isArray(value)){for(const v of value.slice(0,100))read(v,depth+1);}
    else if(value&&typeof value==='object'){for(const v of Object.values(value).slice(0,100))read(v,depth+1);}
  }
  function walk(node,depth=0) {
    if(!node||depth>25||visited++>6000)return;
    if(node.nodeType===1) {
      const tag=node.tagName.toLowerCase();
      if(tag.includes('player')||tag==='video'||tag==='source') {
        for(const name of ['src','source','playback-media-json','packaged-media-json','data-video-url','data-mpd','data-hls-url'])read(node.getAttribute(name));
        if(tag==='video')read(node.currentSrc);
      }
      if(tag==='script'&&node.type==='application/ld+json'&&node.textContent.length<200000)read(node.textContent);
      if(node.shadowRoot)walk(node.shadowRoot,depth+1);
    }
    for(const child of node.children||[])walk(child,depth+1);
  }
  if(area)walk(area);
  const resources=performance.getEntriesByType('resource').slice(-250).map(e=>e.name).filter(x=>x.startsWith('https://v.redd.it/')).slice(-60);
  const title=(root?.getAttribute('post-title')||document.querySelector('h1')?.textContent||document.title||'Reddit video').trim().slice(0,300);
  return {page_url:location.href,title,dom_urls:[...urls],resource_urls:resources,matched:!!root};
})();
