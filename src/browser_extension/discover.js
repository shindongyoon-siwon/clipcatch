// On-demand DOM-only discovery. No cookies, private APIs, automatic scrolling,
// page globals, hidden JSON blobs, request headers or network reads.
(() => {
 const MAX_POSTS=40,MAX_NODES=8000,MAX_URLS=60;
 let visited=0;
 const out=[],host=location.hostname.toLowerCase(),is=d=>host===d||host.endsWith('.'+d);
 const text=x=>String(x||'').replace(/\s+/g,' ').trim().slice(0,250);
 function href(raw){try{const u=new URL(raw,location.href);return u.protocol==='https:'&&!u.username&&!u.password&&!u.port?u.href:'';}catch{return '';}}
 function shown(el){const r=el.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(el).visibility!=='hidden'&&getComputedStyle(el).display!=='none';}
 function tree(n,fn,depth=0){if(!n||depth>24||visited++>MAX_NODES)return;if(fn(n)===false)return;if(n.shadowRoot)tree(n.shadowRoot,fn,depth+1);for(const c of n.children||[])tree(c,fn,depth+1);}
 function media(root){
  const urls=new Set();let count=0;
  function read(s){if(typeof s!=='string'||s.length>180000||urls.size>=MAX_URLS)return;s=s.replace(/\\\//g,'/').replace(/&amp;/g,'&');for(const m of s.matchAll(/https:\/\/v\.redd\.it\/[a-z0-9]{5,40}\/[A-Za-z0-9_-]+\.(?:mpd|m3u8|mp4)(?:\?[^\s"'<>\\]*)?/g)){if(urls.size<MAX_URLS)urls.add(m[0]);}}
  tree(root,n=>{
   if(n!==root&&n.matches?.('shreddit-post,.thing[data-fullname],[data-testid="post-container"]'))return false;
   const tag=(n.tagName||'').toLowerCase();
   if(tag==='video'||tag==='shreddit-player'||tag==='shreddit-video'||tag==='video-player')count++;
   if(tag==='video'||tag==='source'||tag.includes('player')){for(const a of ['src','source','playback-media-json','packaged-media-json','data-video-url','data-mpd','data-hls-url'])read(n.getAttribute(a));if(tag==='video')read(n.currentSrc);}
  });return {dom_urls:[...urls],video_count:count};
 }
 function add(url,title,root,extra={}){if(out.length>=MAX_POSTS||!url)return;out.push({page_url:href(url),title:text(title),...extra});}
 if(is('reddit.com')){
  const roots=[...document.querySelectorAll('shreddit-post,.thing[data-fullname],article[data-testid="post-container"],[data-testid="post-container"]')];
  for(const root of roots.filter((r,i)=>!roots.some((p,j)=>j!==i&&p.contains(r)))){
   if(out.length>=MAX_POSTS)break;if(!shown(root))continue;
   const link=root.getAttribute('permalink')||root.getAttribute('content-href')||root.querySelector('a[slot="full-post-link"],a[data-click-id="body"],a.comments,a[data-testid="post-title"]')?.getAttribute('href')||[...root.querySelectorAll('a[href]')].map(a=>a.getAttribute('href')).find(s=>/\/comments\/[a-z0-9]{5,12}(?:\/|$)/i.test(s||''));
   if(!link)continue;const m=media(root),kind=(root.getAttribute('post-type')||root.getAttribute('data-type')||'').toLowerCase();
   if(!m.dom_urls.length&&!m.video_count&&!['video','gif','rich:video'].includes(kind))continue;
   add(link,root.getAttribute('post-title')||root.querySelector('h1,h2,h3,a.title,[slot="title"]')?.textContent||'Reddit video',root,{...m,matched:true});
  }
 }else if(is('youtube.com')){
  for(const root of document.querySelectorAll('ytd-video-renderer,ytd-grid-video-renderer,ytd-rich-item-renderer,ytd-reel-item-renderer,ytm-video-with-context-renderer')){if(out.length>=MAX_POSTS)break;if(!shown(root))continue;const a=root.querySelector('a#video-title,a#video-title-link,a[href*="/shorts/"],a[href*="/watch?"]');if(a)add(a.href,a.getAttribute('title')||a.textContent,root,{matched:true});}
 }else if(is('x.com')||is('twitter.com')){
  for(const root of document.querySelectorAll('article[data-testid="tweet"]')){
   if(out.length>=MAX_POSTS)break;if(!shown(root)||!root.querySelector('video,[data-testid="videoPlayer"]'))continue;
   const a=root.querySelector('time')?.closest('a[href]');if(!a)continue;
   const title=root.querySelector('[data-testid="tweetText"]')?.textContent||'X video',status=(a.href.match(/\/status\/([0-9]+)/)||[])[1];
   const indexed=[...new Set([...root.querySelectorAll('a[href]')].map(x=>x.href).filter(x=>{const m=x.match(/\/status\/([0-9]+)\/video\/([1-9][0-9]?)(?:[/?#]|$)/);return m&&m[1]===status;}))];
   if(indexed.length>1){for(const url of indexed)add(url,title,root,{matched:true,indexed:true});}else add(a.href,title,root,{matched:true});
  }
 }else if(is('instagram.com')||is('facebook.com')){
  for(const root of document.querySelectorAll('article,[role="article"]')){if(out.length>=MAX_POSTS)break;if(!shown(root)||!root.querySelector('video'))continue;const links=[...root.querySelectorAll('a[href]')],a=links.find(a=>a.querySelector('time'))||links.find(a=>/\/(p|reel|videos)\//.test(a.getAttribute('href')||''));if(a)add(a.href,root.querySelector('h1,h2,h3')?.textContent||root.innerText?.slice(0,220),root,{matched:true});}
 }
 return {page_url:location.href,title:text(document.title),records:out,limited:out.length>=MAX_POSTS,visited};
})();
