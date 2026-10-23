(() => {
  const byId = id => document.getElementById(id);
  const status = message => { byId('browserLinkStatus').textContent = message; };
  async function api(path, method='GET') {
    const r = await fetch(path,{method,headers:method==='POST'?{'Content-Type':'application/json'}:{},body:method==='POST'?'{}':undefined,cache:'no-store'});
    const data=await r.json();if(!r.ok)throw Error(data.error||'ClipCatch connection failed');return data;
  }
  async function refresh() {
    try {const data=await api('/api/browser-link/status');
      byId('browserExtensionPath').textContent=data.extension_path;
      status(data.paired?'브라우저 연결됨 · Reddit 게시물에서 확장을 열어 영상을 보내 줘.':'브라우저 연결 전 · 확장 설치 후 연결 코드를 붙여넣어 줘.');
    }catch(e){status(e.message);}
  }
  byId('installBrowserExtension').onclick=async()=>{try{const d=await api('/api/browser-link/install','POST');byId('browserExtensionPath').textContent=d.path;status('확장 파일 준비 완료. Chrome 확장 관리에서 이 폴더를 선택해 줘.');}catch(e){status(e.message);}};
  byId('makeBrowserCode').onclick=async()=>{try{const d=await api('/api/browser-link/code','POST');byId('browserCodeBox').hidden=false;byId('browserPairCode').value=d.endpoint+'#'+d.code;status('연결 코드를 확장에 붙여넣어 줘. 이 코드는 다른 사람에게 공유하지 마.');}catch(e){status(e.message);}};
  byId('copyBrowserCode').onclick=async()=>{const el=byId('browserPairCode');try{await navigator.clipboard.writeText(el.value);status('연결 코드 복사 완료');}catch(_){el.focus();el.select();status('Ctrl+C로 복사해 줘.');}};
  byId('revokeBrowserLink').onclick=async()=>{try{await api('/api/browser-link/disconnect','POST');byId('browserCodeBox').hidden=true;byId('browserPairCode').value='';await refresh();}catch(e){status(e.message);}};
  byId('browserLinkPanel').addEventListener('toggle',()=>{if(byId('browserLinkPanel').open)refresh();});
  window.addEventListener('focus',refresh);
  refresh();
})();
