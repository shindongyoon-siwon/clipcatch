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
      status(data.paired?'\ube0c\ub77c\uc6b0\uc800 \uc5f0\uacb0\ub428 \u00b7 Reddit \uac8c\uc2dc\ubb3c\uc5d0\uc11c \ud655\uc7a5\uc744 \uc5f4\uc5b4 \uc601\uc0c1\uc744 \ubcf4\ub0b4 \uc918.':'\ube0c\ub77c\uc6b0\uc800 \uc5f0\uacb0 \uc804 \u00b7 \ud655\uc7a5 \uc124\uce58 \ud6c4 \uc5f0\uacb0 \ucf54\ub4dc\ub97c \ubd99\uc5ec\ub123\uc5b4 \uc918.');
    }catch(e){status(e.message);}
  }
  byId('installBrowserExtension').onclick=async()=>{try{const d=await api('/api/browser-link/install','POST');byId('browserExtensionPath').textContent=d.path;status('\ud655\uc7a5 \ud30c\uc77c \uc900\ube44 \uc644\ub8cc. Chrome \ud655\uc7a5 \uad00\ub9ac\uc5d0\uc11c \uc774 \ud3f4\ub354\ub97c \uc120\ud0dd\ud574 \uc918.');}catch(e){status(e.message);}};
  byId('makeBrowserCode').onclick=async()=>{try{const d=await api('/api/browser-link/code','POST');byId('browserCodeBox').hidden=false;byId('browserPairCode').value=d.endpoint+'#'+d.code;status('\uc5f0\uacb0 \ucf54\ub4dc\ub97c \ud655\uc7a5\uc5d0 \ubd99\uc5ec\ub123\uc5b4 \uc918. \uc774 \ucf54\ub4dc\ub294 \ub2e4\ub978 \uc0ac\ub78c\uc5d0\uac8c \uacf5\uc720\ud558\uc9c0 \ub9c8.');}catch(e){status(e.message);}};
  byId('copyBrowserCode').onclick=async()=>{const el=byId('browserPairCode');try{await navigator.clipboard.writeText(el.value);status('\uc5f0\uacb0 \ucf54\ub4dc \ubcf5\uc0ac \uc644\ub8cc');}catch(_){el.focus();el.select();status('Ctrl+C\ub85c \ubcf5\uc0ac\ud574 \uc918.');}};
  byId('revokeBrowserLink').onclick=async()=>{try{await api('/api/browser-link/disconnect','POST');byId('browserCodeBox').hidden=true;byId('browserPairCode').value='';await refresh();}catch(e){status(e.message);}};
  byId('browserLinkPanel').addEventListener('toggle',()=>{if(byId('browserLinkPanel').open)refresh();});
  refresh();
})();
