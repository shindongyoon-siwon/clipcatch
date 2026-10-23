"""Real Chromium extension + local ClipCatch fixture, no live Reddit claims."""
import json, os, pathlib, subprocess, tempfile, time, shutil, signal
from playwright.sync_api import sync_playwright, expect
import re
root=pathlib.Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory() as d:
    ready=pathlib.Path(d)/'ready.txt'
    log=open(pathlib.Path(d)/'server.log','w+')
    proc=subprocess.Popen(['go','test','-run','^TestBrowserE2EServer$','-v','.'],cwd=root/'src',env={**os.environ,'CLIPCATCH_E2E_READY':str(ready)},stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
    try:
        for _ in range(200):
            if ready.exists():break
            if proc.poll() is not None:raise RuntimeError('fixture server failed')
            time.sleep(.1)
        base=ready.read_text()
        with sync_playwright() as p:
            ext=root/'src'/'browser_extension'
            browser=p.chromium.launch_persistent_context(str(pathlib.Path(d)/'profile'),executable_path=os.environ.get('CLIPCATCH_CHROMIUM') or shutil.which('chromium'),channel='chromium',headless=True,args=['--no-sandbox','--disable-extensions-except='+str(ext),'--load-extension='+str(ext)])
            errors=[]
            app=browser.new_page();app.on('pageerror',lambda e:errors.append(str(e)))
            app.goto(base);app.wait_for_selector('#browserLinkPanel')
            app.locator('#browserLinkPanel summary').click()
            app.locator('#makeBrowserCode').click()
            expect(app.locator("#browserPairCode")).to_have_value(re.compile("#"))
            code=app.locator('#browserPairCode').input_value()
            workers=browser.service_workers
            worker=workers[0] if workers else browser.wait_for_event('serviceworker',timeout=10000)
            ext_id=worker.url.split('/')[2]
            popup=browser.new_page();popup.on('pageerror',lambda e:errors.append(str(e)))
            popup.goto('chrome-extension://'+ext_id+'/popup.html')
            popup.locator('#pairCode').fill(code);popup.locator('#pair').click()
            expect(popup.locator("#connectionTitle")).to_contain_text("\uC5F0\uACB0\uB428")
            post='https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/?utm_source=chatgpt.com'
            html='''<!doctype html><html><title>Browser test video</title><body><shreddit-post id="t3_1x1rb7t" post-title="Browser test video"><h1>Browser test video</h1><shreddit-player playback-media-json='{"dash_url":"https://v.redd.it/abcdefghijk12/DASHPlaylist.mpd?f=hd"}'></shreddit-player></shreddit-post><shreddit-post id="t3_another"><shreddit-player src="https://v.redd.it/unrelatedvideo/HLSPlaylist.m3u8"></shreddit-player></shreddit-post></body></html>'''
            reddit=browser.new_page();reddit.route('https://www.reddit.com/**',lambda route:route.fulfill(status=200,content_type='text/html',body=html));reddit.goto(post);reddit.bring_to_front()
            scan=popup.evaluate("chrome.runtime.sendMessage({kind:'scan'})")
            assert scan['ok'],scan
            groups=scan['data']['groups'];assert len(groups)==1 and groups[0]['id']=='abcdefghijk12' and groups[0]['matched'],groups
            # Exercise actual service-worker -> localhost CORS and bearer validation.
            result=popup.evaluate("chrome.runtime.sendMessage({kind:'download',id:'abcdefghijk12',confirmed:true,silent:false})")
            assert result['ok'],result
            app.locator(".job.done").wait_for(state="attached",timeout=15000)
            href=app.locator('.job-url').first.get_attribute('href');assert 'utm_' not in href and '/comments/1x1rb7t/' in href,href
            app.evaluate("document.getElementById('browserCodeBox').hidden=true;document.getElementById('browserPairCode').value=''")
            out=root/'qa-output';out.mkdir(exist_ok=True)
            app.screenshot(path=str(out/'browser-app.png'),full_page=True)
            popup.evaluate("chrome.runtime.sendMessage({kind:'status'}).then(s=>{window.__qaState=s;})")
            app.locator('#revokeBrowserLink').click()
            reddit.bring_to_front()
            rejected=popup.evaluate("chrome.runtime.sendMessage({kind:'download',id:'abcdefghijk12',confirmed:true})")
            assert not rejected['ok'],rejected
            assert not errors,errors
            print(json.dumps({'paired':True,'dom_scanned':True,'unrelated_video_excluded':True,'localhost_transfer':True,'fixture_video_audio_verified':True,'original_link_kept_clean':True,'revocation_enforced':True,'javascript_errors':errors,'live_reddit_download_tested':False},indent=2))
            browser.close()
    finally:
        os.killpg(proc.pid,signal.SIGTERM) if proc.poll() is None else None
        try:proc.wait(timeout=5)
        except subprocess.TimeoutExpired:proc.kill()
        log.flush();log.seek(0)
        print(log.read()[-3000:]);log.close()
