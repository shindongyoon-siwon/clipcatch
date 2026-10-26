import test from 'node:test';import assert from 'node:assert/strict';
const local={},session={},calls=[];let listener,active={id:11,url:'https://www.reddit.com/r/aww/'},records=[];
const area=d=>({get:async k=>typeof k==='string'?{[k]:d[k]}:Object.fromEntries(k.map(x=>[x,d[x]])),set:async v=>Object.assign(d,structuredClone(v)),remove:async k=>{delete d[k];}});
globalThis.chrome={runtime:{id:'a'.repeat(32),getURL:p=>'chrome-extension://'+'a'.repeat(32)+'/'+p,onMessage:{addListener:f=>listener=f}},tabs:{query:async()=>[active],get:async()=>active,onRemoved:{addListener:()=>{}}},scripting:{executeScript:async()=>[{result:{page_url:active.url,title:'fixture',records}}]},storage:{local:area(local),session:area(session)},webRequest:{onCompleted:{addListener:()=>{}}}};
let failedFirst=false,changeTab=false;
let simulatedBackend='clipcatch', jobState='queued';
globalThis.fetch=async(url,opt)=>{
 const b=JSON.parse(opt.body);
 if(url.endsWith('/bridge/status'))return new Response(JSON.stringify({app:simulatedBackend,channel:'stable',version:'1.9.1-test.2',capabilities:['job-status','output-mode'],jobs:(b.ids||[]).map(id=>({id,status:jobState,stage:'converting',percent:38,output_mode:'premiere',file_count:jobState==='done'?2:0}))}),{status:200});
 calls.push({url,b});
 if(url.endsWith('/bridge/pair'))return new Response(JSON.stringify({protocol:1,token:'b'.repeat(64),version:'1.9.1-test.2',channel:'stable'}),{status:200});
 if(changeTab){active={...active,url:'https://www.reddit.com/r/new/'};changeTab=false;}
 if(failedFirst){failedFirst=false;return new Response('{"error":"fixture failure"}',{status:400});}
 return new Response(JSON.stringify({ok:true,count:1,job_ids:['job-'+calls.length]}),{status:200});
};
await import('../src/browser_extension/background.js');
const sender={id:chrome.runtime.id,url:chrome.runtime.getURL('popup.html')},ask=(m,s=sender)=>new Promise(resolve=>listener(m,s,resolve));
const feed='https://www.reddit.com/r/aww/',post='https://www.reddit.com/comments/abc123/',media=['https://v.redd.it/firstmedia123/DASHPlaylist.mpd','https://v.redd.it/secondmedia123/DASHPlaylist.mpd'];
async function setup(){active={id:11,url:feed};records=[{page_url:post,title:'two',matched:true,dom_urls:media}];calls.length=0;local.recentTransfers=[];simulatedBackend='clipcatch';jobState='queued';local.link={base:'http://127.0.0.1:8765',token:'b'.repeat(64)};return(await ask({kind:'list'})).data.candidates.map(c=>c.key);}
test('no network until explicit send',async()=>{assert.equal((await setup()).length,2);assert.equal(calls.length,0);});
test('two selected streams send separately',async()=>{const keys=await setup(),r=await ask({kind:'send-selected',keys});assert.equal(r.ok,true);assert.ok(r.data.results.every(x=>x.ok));assert.equal(r.data.results.length,2);assert.deepEqual(calls.map(x=>x.b.media_url),media);assert.ok(calls.every(x=>x.b.separate===true));});
test('partial failure stays partial',async()=>{const keys=await setup();failedFirst=true;const r=await ask({kind:'send-selected',keys});assert.equal(r.data.results[0].ok,false);assert.equal(r.data.results[1].ok,true);});
test('changed page blocks old selection',async()=>{const keys=await setup();active.url='https://www.reddit.com/r/new/';assert.equal((await ask({kind:'send-selected',keys})).ok,false);assert.equal(calls.length,0);});
test('forged choice and sender denied',async()=>{await setup();assert.equal((await ask({kind:'send-selected',keys:['forged']})).ok,false);assert.equal((await ask({kind:'list'},{id:sender.id,url:'https://evil.test/'})).ok,false);assert.equal(calls.length,0);});
test('expired or oversized selection denied',async()=>{const keys=await setup();session.collection.started=Date.now()-700000;assert.equal((await ask({kind:'send-selected',keys})).ok,false);assert.equal((await ask({kind:'send-selected',keys:Array(11).fill(keys[0])})).ok,false);assert.equal(calls.length,0);});
test('tab change interrupts remaining batch',async()=>{const keys=await setup();changeTab=true;const r=await ask({kind:'send-selected',keys});assert.equal(r.data.results[0].ok,true);assert.equal(r.data.results[1].ok,false);assert.equal(calls.length,1);});
test('video-only requires consent',async()=>{await setup();records=[{page_url:post,matched:true,dom_urls:['https://v.redd.it/firstmedia123/DASH_720.mp4']}];const keys=(await ask({kind:'list'})).data.candidates.map(c=>c.key);const r=await ask({kind:'send-selected',keys});assert.equal(r.data.results[0].ok,false);assert.equal(calls.length,0);});

test('MP4 selection reaches every Reddit media request',async()=>{const keys=await setup();await ask({kind:'output-mode',value:'original'});const r=await ask({kind:'send-selected',keys,output_mode:'premiere'});assert.ok(r.data.results.every(x=>x.ok));assert.ok(calls.every(x=>x.b.output_mode==='premiere'));assert.equal(local.outputMode,'original');});
test('stored original choice and explicit original are respected',async()=>{let keys=await setup();await ask({kind:'output-mode',value:'original'});await ask({kind:'send-selected',keys});assert.ok(calls.every(x=>x.b.output_mode==='original'));keys=await setup();await ask({kind:'send-selected',keys,output_mode:'original'});assert.ok(calls.every(x=>x.b.output_mode==='original'));});
test('invalid output mode never queues',async()=>{const keys=await setup();const r=await ask({kind:'send-selected',keys,output_mode:'bad'});assert.equal(r.ok,false);assert.equal(calls.length,0);});
test('generic post handoff includes MP4 mode',async()=>{await setup();active={id:11,url:'https://www.youtube.com/watch?v=abcdefghijk'};await ask({kind:'send-page',output_mode:'premiere'});assert.equal(calls[0].b.output_mode,'premiere');assert.ok(calls[0].url.endsWith('/bridge/page'));});
test('stable extension refuses TEST2 pairing before any request',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:18766#'+'a'.repeat(64)});assert.equal(r.ok,false);assert.equal(calls.length,0);});
test('stable pairing is explicitly marked and accepted',async()=>{await setup();const r=await ask({kind:'pair',value:'http://127.0.0.1:8765#'+'a'.repeat(64)});assert.equal(r.ok,true);assert.equal(calls[0].b.channel,'stable');});

test('popup can observe queued converting and done without a web page',async()=>{const keys=await setup();await ask({kind:'send-selected',keys,output_mode:'premiere'});assert.equal(local.recentTransfers.length,2);jobState='downloading';let r=await ask({kind:'queue-status'});assert.equal(r.data.jobs[0].stage,'converting');assert.equal(r.data.jobs[0].percent,38);jobState='done';r=await ask({kind:'queue-status'});assert.equal(r.data.jobs[0].file_count,2);assert.equal(r.data.jobs[0].status,'done');});
test('wrong running app blocks downloads even with stored pairing',async()=>{const keys=await setup();simulatedBackend='clipcatch-test2';const r=await ask({kind:'send-selected',keys});assert.equal(r.ok,false);assert.equal(calls.length,0);});
test('TEST2 and TEST1 pairing never contact either backend',async()=>{await setup();for(const port of [18766,18765]){const r=await ask({kind:'pair',value:'http://127.0.0.1:'+port+'#'+'a'.repeat(64)});assert.equal(r.ok,false);}assert.equal(calls.length,0);});

test('worker exposes exact build before popup sends new commands',async()=>{await setup();const r=await ask({kind:'hello'});assert.equal(r.ok,true);assert.equal(r.data.build,'1.9.1');assert.equal(r.data.channel,'stable');assert.equal(calls.length,0);});
