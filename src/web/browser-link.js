(() => {
 const $=id=>document.getElementById(id);
 const status=t=>{$('browserLinkStatus').textContent=t;};
 async function api(path,method='GET'){
  const r=await fetch(path,{method,headers:method==='POST'?{'Content-Type':'application/json'}:{},body:method==='POST'?'{}':undefined,cache:'no-store'});
  const data=await r.json();if(!r.ok)throw Error(data.error||'연결 요청에 실패했습니다.');return data;
 }
 let feedbackUntil=0;
 async function refresh(){if(Date.now()<feedbackUntil)return;try{const d=await api('/api/browser-link/status');status(d.paired?'연결됨 · ClipCatch Browser':'설치 후 연결 코드를 확장프로그램에 붙여넣어 주세요.');}catch(e){status(e.message);}}
 const show=t=>{feedbackUntil=Date.now()+12000;status(t);};
 $('installBrowserExtension').onclick=async()=>{const b=$('installBrowserExtension');b.disabled=true;try{const d=await api('/api/browser-link/install','POST');$('browserExtensionPath').textContent=d.path;$('browserExtensionPath').hidden=!!d.opened;show(d.opened?'설치 폴더를 열었습니다. [설치 방법]에 따라 확장프로그램을 등록해 주세요.':'폴더를 자동으로 열지 못했습니다. 표시된 경로를 파일 탐색기에 붙여넣어 주세요.');}catch(e){show(e.message);}finally{b.disabled=false;}};
 async function copy(){const x=$('browserPairCode');try{await navigator.clipboard.writeText(x.value);$('browserCodeBox').hidden=true;show('연결 코드를 복사했습니다. ClipCatch Browser에 붙여넣어 주세요.');}catch{$('browserCodeBox').hidden=false;x.focus();x.select();show('Ctrl+C로 코드를 복사해 주세요.');}}
 $('makeBrowserCode').onclick=async()=>{const b=$('makeBrowserCode');b.disabled=true;try{const d=await api('/api/browser-link/code','POST');$('browserPairCode').value=d.endpoint+'#'+d.code;await copy();}catch(e){show(e.message);}finally{b.disabled=false;}};
 $('copyBrowserCode').onclick=copy;
 $('revokeBrowserLink').onclick=async()=>{try{await api('/api/browser-link/disconnect','POST');$('browserCodeBox').hidden=true;$('browserPairCode').value='';show('확장프로그램 연결을 해제했습니다.');}catch(e){show(e.message);}};
 $('browserLinkPanel').addEventListener('toggle',()=>{if($('browserLinkPanel').open)refresh();});
 window.addEventListener('focus',refresh);refresh();
})();
