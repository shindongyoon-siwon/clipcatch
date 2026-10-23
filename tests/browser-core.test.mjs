import test from 'node:test';
import assert from 'node:assert/strict';
import {videoPage,redditPost,mediaParts,groupMedia,pairingCode} from '../src/browser_extension/core.mjs';
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

test('multi-site extension accepts explicit video posts across platforms',()=>{
 const samples=[
  ['https://www.youtube.com/watch?v=abcdefghijk','YouTube'],
  ['https://youtube.com/shorts/abcdefghijk?utm_source=test','YouTube'],
  ['https://youtu.be/abcdefghijk','YouTube'],
  ['https://x.com/someone/status/12345678','X (트위터)'],
  ['https://twitter.com/user/status/12345678','X (트위터)'],
  ['https://www.instagram.com/reel/AbcDEF-123/','Instagram'],
  ['https://www.instagram.com/p/AbcDEF-123/','Instagram'],
  ['https://www.reddit.com/r/aww/comments/abc123/title/','Reddit'],
  ['https://www.tiktok.com/@creator/video/12345678','TikTok'],
  ['https://fb.watch/abcDEFG/','Facebook'],
  ['https://www.vimeo.com/123456','Vimeo'],
  ['https://streamable.com/AbC123','Streamable'],
  ['https://www.twitch.tv/videos/123456','Twitch'],
  ['https://imgur.com/a/A1BcD','Imgur']
 ];
 for(const [url,platform]of samples){
  const page=videoPage(url);assert.ok(page,'rejected '+url);
  assert.equal(page.platform,platform,url);assert.ok(page.url.startsWith('https://'));
  assert.ok(!page.url.includes('utm_source'));
 }
});
test('multi-site extension rejects feeds, credentials, local URLs and lookalike domains',()=>{
 for(const url of [
  'https://youtube.com/','https://youtube.com/results?search_query=cat',
  'https://youtube.com/@creator','https://instagram.com/creator/',
  'https://x.com/home','https://reddit.com/r/aww/',
  'https://www.tiktok.com/@creator','https://www.facebook.com/',
  'https://youtube.com.evil.test/watch?v=abcdefghijk',
  'https://user:pass@www.youtube.com/watch?v=abcdefghijk',
  'http://www.youtube.com/watch?v=abcdefghijk',
  'https://127.0.0.1:8765','file:///test',
  'https://youtu.be/not-an-11charid',
 ])assert.equal(videoPage(url),null,url);
});
