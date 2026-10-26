(() => {
 'use strict';
 const $=id=>document.getElementById(id),el=(tag,text='')=>{const n=document.createElement(tag);n.textContent=text;return n;};
 const style=el('style');style.textContent=`
 .navbar{padding-bottom:14px!important}.trial-banner{margin:14px 0;padding:12px 16px;border:1px solid #6c7349;border-radius:10px;color:#dce9bb;background:#252c20;line-height:1.55;font-size:12px}
 .trial-banner strong{font-size:13px;display:block;margin-bottom:3px}
 .workspace-tabs{display:flex;flex-wrap:wrap;gap:8px;margin:20px 0 16px}
 .workspace-tabs button{padding:12px 18px;border:1px solid #485564;border-radius:10px;color:#e8edf3;background:#1c2733;cursor:pointer;font:inherit;font-size:14px;font-weight:700}
 .workspace-tabs button[aria-selected=true]{background:#d9ff57;color:#17200d;border-color:#d9ff57}
 .workspace-tabs button:focus-visible{outline:2px solid #d9ff57;outline-offset:3px}
 #qualityPanel,#browserLinkPanel{margin-top:0;padding:18px;border:1px solid #45515f;border-radius:12px;background:#17212a}
 #qualityPanel>summary,#browserLinkPanel>summary{list-style:none;pointer-events:none;font-size:17px;margin-bottom:16px}
 #qualityPanel>summary::-webkit-details-marker,#browserLinkPanel>summary::-webkit-details-marker{display:none}
 #browserInstallHelp{border-top:1px solid #44505d;margin-top:16px;padding-top:12px}
 #browserInstallHelp summary{cursor:pointer;font-size:13px}
 #browserInstallHelp ol{padding-left:22px;font-size:13px;line-height:1.9}
 #browserLinkPanel p{line-height:1.7;font-size:13px}
 [hidden]{display:none!important}
 .hero{padding:12px 0 2px!important;margin:0!important}.hero h1{font-size:28px!important;line-height:1.25!important;letter-spacing:-1px!important;margin:9px 0!important}
 .hero h1 br{display:none}.hero h1 em{margin-left:10px}.hero p{font-size:13px!important;line-height:1.6!important;margin:8px 0 12px!important}
 @media(max-width:650px){.workspace-tabs button{flex:1 1 auto;padding:10px;font-size:12px}.hero h1{font-size:23px!important}.hero h1 em{display:block;margin-left:0}.trial-banner{font-size:11px}}
 `;document.head.append(style);
 const hero=document.querySelector('.hero'),download=document.querySelector('.main-card'),quality=$('qualityPanel'),extension=$('browserLinkPanel');
 if(!hero||!download||!quality||!extension)return;
 download.id='downloadPanel';
 const banner=el('div');banner.className='trial-banner';banner.id='trialBanner';banner.append(el('strong','ClipCatch v1.9.2 · 영상 다운로드·화질 개선'));
 const identity=el('span','운영판 확인 중');identity.id='buildIdentity';banner.append(identity);
 const share=el('button','운영판 공유 링크');share.type='button';share.id='shareStable';share.style.cssText='margin-left:12px;padding:7px 10px;border:1px solid #60754b;border-radius:7px;background:#293426;color:#d9ff57;cursor:pointer';banner.append(share);
 const shareText=el('input');shareText.readOnly=true;shareText.hidden=true;shareText.setAttribute('aria-label','팀 공유용 운영판 다운로드 링크');shareText.style.cssText='width:100%;margin-top:10px;padding:8px';banner.append(shareText);
 const stableURL='https://github.com/shindongyoon-siwon/clipcatch/releases/latest/download/ClipCatch.exe';
 share.onclick=async()=>{shareText.value=stableURL;shareText.hidden=false;try{await navigator.clipboard.writeText(stableURL);share.textContent='공유 링크 복사됨';}catch(_){shareText.focus();shareText.select();}};
 const oldRender=render;render=function(st){oldRender(st);if(st){const trial=st.channel==='test';identity.textContent=(trial?'TEST2 테스트판 · 이 EXE를 팀에 배포하지 마세요.':'운영판 · 팀에는 오른쪽 공유 링크를 전달해 주세요.');banner.querySelector('strong').textContent='ClipCatch v'+st.version+(trial?' TEST2':' 운영판');}};

 hero.before(banner);hero.hidden=true;
 const tabs=el('nav');tabs.className='workspace-tabs';tabs.setAttribute('role','tablist');tabs.setAttribute('aria-label','작업 선택');hero.after(tabs);
 const entries=[['navDownload','영상 다운로드',download],['navQuality','이미지·영상 화질 개선',quality],['navExtension','확장프로그램 다운로드',extension]];
 // Move the existing real panels; no new mock feature or duplicate settings.
 download.after(quality,extension);
 let active='navDownload';const buttons=[];
 const activate=id=>{active=id;for(let i=0;i<entries.length;i++){const [key,,panel]=entries[i];panel.hidden=key!==id;panel.setAttribute('role','tabpanel');panel.setAttribute('aria-labelledby',key);if(key===id&&panel.tagName==='DETAILS')panel.open=true;buttons[i].setAttribute('aria-selected',String(key===id));buttons[i].tabIndex=key===id?0:-1;}};
 entries.forEach(([id,label,panel],index)=>{const b=el('button',label);b.id=id;b.type='button';b.setAttribute('role','tab');b.setAttribute('aria-controls',panel.id);b.onclick=()=>activate(id);b.onkeydown=e=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key))return;e.preventDefault();let next=e.key==='Home'?0:e.key==='End'?2:(index+(e.key==='ArrowRight'?1:2))%3;activate(entries[next][0]);buttons[next].focus();};buttons.push(b);tabs.append(b);});
 activate(active);
 ['checkAppUpdate','installAppUpdate','installAppUpdateFooter','appUpdate','updateFeedback'].forEach(id=>{const n=$(id);if(n)n.hidden=false;});
 const check=$('checkAppUpdate');if(check){check.textContent='새 버전 확인';banner.append(check);}
 if($('appUpdate'))banner.after($('appUpdate'));

 const f=el('span','새 운영 버전은 확인 후 설치할 수 있습니다.');f.style.cssText='font-size:11px;color:#9caebb';document.querySelector('.footer-tools')?.append(f);
})();
