(() => {
 'use strict';
 const $=id=>document.getElementById(id),el=(tag,text='')=>{const n=document.createElement(tag);n.textContent=text;return n;};
 const style=el('style');style.textContent=`
 .navbar{padding-bottom:10px!important}
 .workspace-tabs{display:flex;flex-wrap:wrap;gap:8px;margin:16px 0}
 .workspace-tabs button{padding:11px 17px;border:1px solid #485564;border-radius:10px;color:#e8edf3;background:#1c2733;cursor:pointer;font:inherit;font-size:14px;font-weight:700}
 .workspace-tabs button[aria-selected=true]{background:#d9ff57;color:#17200d;border-color:#d9ff57}
 .workspace-tabs button:focus-visible{outline:2px solid #d9ff57;outline-offset:3px}
 .build-channel{font-size:11px;color:#ffc775;border:1px solid #75592f;border-radius:6px;padding:4px 7px;letter-spacing:0}
 #qualityPanel,#browserLinkPanel{margin-top:0;padding:20px;border:1px solid #45515f;border-radius:16px;background:#17212a}
 #qualityPanel>summary,#browserLinkPanel>summary{list-style:none;pointer-events:none;font-size:17px;margin-bottom:16px}
 #qualityPanel>summary::-webkit-details-marker,#browserLinkPanel>summary::-webkit-details-marker{display:none}
 #browserInstallHelp{border-top:1px solid #44505d;margin-top:16px;padding-top:12px}
 #browserInstallHelp summary{cursor:pointer;font-size:13px}
 #browserInstallHelp ol{padding-left:22px;font-size:13px;line-height:1.9}
 #browserLinkPanel p{line-height:1.7;font-size:13px}
 .settings-panel{margin-top:24px;border-top:1px solid #303b4a;color:#aab7c6}
 .settings-panel>summary{padding:18px 0;cursor:pointer;font-size:13px;font-weight:700;width:fit-content}
 .settings-body{padding:0 0 10px}
 .settings-program{padding:16px;border:1px solid #354252;border-radius:12px;background:#141c25}
 .settings-program .footer-tools{gap:8px}
 .settings-program .update-btn{margin:0;font-size:12px;padding:10px 12px}
 .settings-program .app-update{margin:12px 0 0}
 .settings-program .update-feedback{max-width:none}
 .settings-panel .help-panel,.settings-panel .reddit-auth{margin:12px 0 0;border-radius:12px}
 .settings-panel .help-panel>summary{font-size:13px;padding:14px 16px}
 .settings-panel .help-body{padding:14px 16px}
 .settings-note{display:block;font-size:11px;color:#728195;margin:14px 0 0;line-height:1.6}
 .share-url{width:100%;margin-top:10px;padding:10px;background:#101720;border:1px solid #475565;border-radius:8px;color:#d9e4ef;font-size:12px}
 .share-feedback{font-size:12px;color:#c5dcad;margin:10px 0 0}
 [hidden]{display:none!important}
 @media(max-width:650px){.workspace-tabs button{flex:1 1 auto;padding:10px 8px;font-size:12px}.topright{gap:9px}.build-channel{font-size:10px}.settings-program{padding:13px}.help-status{width:100%}}
 `;document.head.append(style);
 const download=document.querySelector('.main-card'),quality=$('qualityPanel'),extension=$('browserLinkPanel');
 if(!download||!quality||!extension)return;
 download.id='downloadPanel';

 // Only a TEST2 build needs a channel label; stable already has one version in the header.
 const identity=el('span');identity.id='buildIdentity';identity.className='build-channel';identity.hidden=true;document.querySelector('.brand')?.append(identity);
 const oldRender=render;render=function(st){oldRender(st);if(st){const trial=st.channel==='test';identity.hidden=!trial;identity.textContent=trial?'TEST2 테스트판':'';identity.title=trial?'팀 공유는 설정의 최신 다운로드 링크를 이용해 주세요.':'';}};

 const tabs=el('nav');tabs.className='workspace-tabs';tabs.setAttribute('role','tablist');tabs.setAttribute('aria-label','작업 선택');download.before(tabs);
 const entries=[['navDownload','영상 다운로드',download],['navQuality','화질 개선',quality],['navExtension','확장프로그램',extension]];
 // Move the existing real panels and preserve their listeners and settings.
 download.after(quality,extension);
 let active='navDownload';const buttons=[];
 const activate=id=>{active=id;for(let i=0;i<entries.length;i++){const [key,,panel]=entries[i];panel.hidden=key!==id;panel.setAttribute('role','tabpanel');panel.setAttribute('aria-labelledby',key);if(key===id&&panel.tagName==='DETAILS')panel.open=true;buttons[i].setAttribute('aria-selected',String(key===id));buttons[i].tabIndex=key===id?0:-1;}};
 entries.forEach(([id,label,panel],index)=>{const b=el('button',label);b.id=id;b.type='button';b.setAttribute('role','tab');b.setAttribute('aria-controls',panel.id);b.onclick=()=>activate(id);b.onkeydown=e=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key))return;e.preventDefault();let next=e.key==='Home'?0:e.key==='End'?entries.length-1:(index+(e.key==='ArrowRight'?1:entries.length-1))%entries.length;activate(entries[next][0]);buttons[next].focus();};buttons.push(b);tabs.append(b);});
 activate(active);

 const settings=el('details');settings.id='settingsPanel';settings.className='settings-panel';settings.append(el('summary','설정 · 도움말'));
 const body=el('div');body.className='settings-body';settings.append(body);
 const program=el('section');program.className='settings-program';program.setAttribute('aria-label','업데이트 및 공유');
 const tools=document.querySelector('.footer-tools');
 const check=$('checkAppUpdate');if(check)check.textContent='업데이트 확인';
 const share=el('button','공유 링크 복사');share.type='button';share.id='shareStable';share.className='update-btn';tools.append(share);
 const shareText=el('input');shareText.readOnly=true;shareText.hidden=true;shareText.className='share-url';shareText.setAttribute('aria-label','팀 공유용 최신 다운로드 링크');
 const shareFeedback=el('p');shareFeedback.className='share-feedback';shareFeedback.hidden=true;shareFeedback.setAttribute('role','status');
 const stableURL='https://github.com/shindongyoon-siwon/clipcatch/releases/latest/download/ClipCatch.exe';
 share.onclick=async()=>{shareText.value=stableURL;shareFeedback.hidden=false;try{await navigator.clipboard.writeText(stableURL);shareText.hidden=true;shareFeedback.textContent='공유 링크를 복사했습니다.';}catch(_){shareText.hidden=false;shareText.focus();shareText.select();shareFeedback.textContent='아래 링크를 Ctrl+C로 복사해 주세요.';}};
 program.append(tools,shareText,shareFeedback,$('updateFeedback'),$('appUpdate'));body.append(program,$('redditAuthDetails'),$('helpPanel'));
 const footer=document.querySelector('.footer'),legal=footer?.querySelector('small');if(legal){legal.className='settings-note';body.append(legal);}footer?.remove();
 $('history').closest('section').after(settings);
 if(lastState)render(lastState);
})();
