import test from 'node:test';
import assert from 'node:assert/strict';
import {redditPost,mediaParts,groupMedia,pairingCode} from '../src/browser_extension/core.mjs';
const base='https://v.redd.it/abcdefghijk12/';
test('exact supplied Reddit post strips UTM without changing ID',()=>{
 const p=redditPost('https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/?utm_source=chatgpt.com');
 assert.equal(p.id,'1x1rb7t');assert.ok(!p.url.includes('utm_'));
 for(const raw of ['https://reddit.com.evil.test/r/cats/comments/abc123/','https://reddit.com/r/cats/','javascript:alert(1)','https://user@reddit.com/comments/abcdef/'])assert.equal(redditPost(raw),null);
});
test('media URL allowlist rejects local, credentials and unsupported content',()=>{
 assert.equal(mediaParts(base+'DASHPlaylist.mpd?f=hd').kind,'manifest');
 assert.equal(mediaParts(base+'DASH_AUDIO_128.mp4').kind,'audio');
 for(const raw of ['http://v.redd.it/abcdefghijk12/DASH_720.mp4','https://v.redd.it.evil.test/abcdefghijk12/DASH_720.mp4','https://u:p@v.redd.it/abcdefghijk12/DASH_720.mp4','https://127.0.0.1/a.mp4',base+'file.json',base+'DASH_720.mp4#x'])assert.equal(mediaParts(raw),null);
});
test('DOM-matched post excludes other recommended videos and pairs exact media ID',()=>{
 const g=groupMedia([base+'DASH_720.mp4'],[base+'DASH_AUDIO_128.mp4','https://v.redd.it/othermedia123/HLSPlaylist.m3u8']);
 assert.equal(g.length,1);assert.equal(g[0].kind,'paired');assert.equal(g[0].matched,true);assert.equal(g[0].audio_url,base+'DASH_AUDIO_128.mp4');
});
test('observed manifest wins; invented manifest is never synthesized',()=>{
 assert.equal(groupMedia([base+'DASH_720.mp4'],[])[0].kind,'video-only');
 const g=groupMedia([base+'HLSPlaylist.m3u8',base+'DASHPlaylist.mpd'],[base+'DASH_720.mp4']);
 assert.equal(g[0].media_url,base+'DASHPlaylist.mpd');assert.equal(g[0].audio_url,'');
});
test('unmatched network is visibly marked and remains separate by ID',()=>{
 const g=groupMedia([],[base+'DASH_720.mp4','https://v.redd.it/othermedia123/DASH_AUDIO_128.mp4']);
 assert.equal(g.length,1);assert.equal(g[0].matched,false);assert.equal(g[0].kind,'video-only');
});
test('pairing permits only local loopback with a full random code',()=>{
 const code='a'.repeat(64);assert.equal(pairingCode('http://127.0.0.1:8765#'+code).base,'http://127.0.0.1:8765');
 for(const raw of ['https://evil.test#'+code,'http://127.0.0.1.evil.test:8765#'+code,'http://user@127.0.0.1:8765#'+code,'http://127.0.0.1:8765/path#'+code,'http://127.0.0.1:8765#short'])assert.throws(()=>pairingCode(raw));
});
