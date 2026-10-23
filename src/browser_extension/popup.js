const $=id=>document.getElementById(id);
const texts={intro:'Reddit \uc6d0\ubcf8 \uac8c\uc2dc\ubb3c\uc5d0\uc11c \uc601\uc0c1\uc744 \uc7ac\uc0dd\ud558\uace0 \uc544\ub798 \ubc84\ud2bc\uc744 \ub20c\ub7ec \uc918.',pairHelp:'ClipCatch\uc758 [\ube0c\ub77c\uc6b0\uc800\uc5d0\uc11c \uac00\uc838\uc624\uae30] \u2192 [\uc5f0\uacb0 \ucf54\ub4dc \ub9cc\ub4e4\uae30]\uc5d0\uc11c \ubcf5\uc0ac\ud55c \ucf54\ub4dc\ub97c \ubd99\uc5ec\ub123\uc5b4 \uc918.',pair:'ClipCatch \uc5f0\uacb0',scan:'\ud604\uc7ac \uac8c\uc2dc\ubb3c \uc601\uc0c1 \ucc3e\uae30',silentLabel:'\uc6d0\ub798 \uc18c\ub9ac\uac00 \uc5c6\ub294 \uc601\uc0c1 \u00b7 \ubb34\uc74c \uc800\uc7a5 \ud5c8\uc6a9',privacy:'\uc120\ud0dd\ud55c Reddit \ud0ed\uc758 \uc601\uc0c1 \uc8fc\uc18c\ub9cc \uc774 PC\ub85c \ubcf4\ub0b4. \ucfe0\ud0a4\u00b7\ube44\ubc00\ubc88\ud638\ub294 \uc77d\uc9c0 \uc54a\uc544. DRM\u00b7\uc811\uadfc \uc81c\ud55c \ud574\uc81c \uae30\ub2a5\uc740 \uc5c6\uc5b4.'};
for(const [id,text] of Object.entries(texts))$(id).textContent=text;
$('pairCode').placeholder='http://127.0.0.1:8765#...';
function notice(text,error=false){$('status').textContent=text;$('status').classList.toggle('error',error);}
async function message(data){const r=await chrome.runtime.sendMessage(data);if(!r?.ok){const e=Error(r?.error||'Extension error');e.duplicate=!!r?.duplicate;throw e;}return r.data;}
function render(state){
  $('connectionTitle').textContent=state.paired?'ClipCatch \uc5f0\uacb0\ub428 \u00b7 \uc7ac\uc5f0\uacb0':'\ucc98\uc74c \ud55c \ubc88 \u00b7 ClipCatch \uc5f0\uacb0';
  if(!state.paired)$('connection').open=true;
  $('pageTitle').textContent=state.title||'';
  $('hint').textContent=state.capturing?'\uc601\uc0c1\uc774 \uc548 \ubcf4\uc774\uba74 \uac8c\uc2dc\ubb3c\uc5d0\uc11c \uc7ac\uc0dd\ud55c \ub4a4 \ub2e4\uc2dc \ucc3e\uc544 \uc918. \uac10\uc9c0\ub294 \uc774 \ud0ed\uc5d0\uc11c\ub9cc 10\ubd84\uac04 \uc720\uc9c0\ub3fc.':'\uc601\uc0c1 \ucc3e\uae30\ub97c \ub20c\ub7ec\uc57c \ud604\uc7ac \ud0ed\uc758 \uac10\uc9c0\uac00 \uc2dc\uc791\ub3fc.';
  $('candidates').replaceChildren();
  for(const g of state.groups||[]){
    const box=document.createElement('div');box.className='candidate';
    const label=document.createElement('b');label.textContent=g.matched?'\ud604\uc7ac \uac8c\uc2dc\ubb3c\uc758 \uc601\uc0c1':'\uc774 \ud0ed\uc5d0\uc11c \uac10\uc9c0\ub41c \ud6c4\ubcf4';
    const info=document.createElement('small');info.textContent=(g.kind==='mpd'||g.kind==='m3u8'?'\ud654\uc9c8\u00b7\uc74c\uc131 \ubaa9\ub85d \uac10\uc9c0':g.kind==='paired'?'\uc601\uc0c1 + \uc74c\uc131 \uc8fc\uc18c \uac10\uc9c0':'\uc601\uc0c1\ub9cc \uac10\uc9c0 \u00b7 \uc74c\uc131 \ubbf8\ud655\uc778')+(g.quality?' \u00b7 \uac10\uc9c0\ub41c '+g.quality+'p':'')+'\nID: '+g.id;
    const button=document.createElement('button');button.textContent='ClipCatch\ub85c \ubcf4\ub0b4 \uc800\uc7a5';button.disabled=!state.paired;
    button.onclick=async()=>{
      const confirmed=g.matched||confirm('\uc774 \ud6c4\ubcf4\uac00 \uc6d0\ubcf8 \uac8c\uc2dc\ubb3c\uc758 \uc601\uc0c1\uc778\uc9c0 \uc790\ub3d9 \ud655\uc815\ud558\uc9c0 \ubabb\ud588\uc5b4. \ud0ed\uc5d0\uc11c \uc6d0\ud558\ub294 \uc601\uc0c1\ub9cc \uc7ac\uc0dd\ud588\ub294\uc9c0 \ud655\uc778\ud588\uc5b4?');
      if(!confirmed)return;
      button.disabled=true;notice('\ubcf4\ub0b4\ub294 \uc911...');
      const data={kind:'download',id:g.id,silent:$('silent').checked,confirmed};
      try{let result;try{result=await message(data);}catch(e){if(!e.duplicate||!confirm(e.message))throw e;result=await message({...data,duplicates:true});}
        notice(result.already_queued?'\uc774\ubbf8 \ub2e4\uc6b4\ub85c\ub4dc \ub300\uae30 \uc911\uc774\uc57c.':'ClipCatch \ub300\uae30\uc5f4\uc5d0 \ubcf4\ub0c8\uc5b4. \uc800\uc7a5 \uc644\ub8cc\ub294 ClipCatch\uc758 \ub2e4\uc6b4\ub85c\ub4dc \ub0b4\uc5ed\uc5d0\uc11c \ud655\uc778\ud574 \uc918.');
      }catch(e){notice(e.message,true);}finally{button.disabled=false;}
    };
    box.append(label,info,button);$('candidates').append(box);
  }
}
$('pair').onclick=async()=>{try{await message({kind:'pair',value:$('pairCode').value});$('pairCode').value='';$('connection').open=false;render(await message({kind:'status'}));notice('\uc5f0\uacb0 \uc644\ub8cc. \uc774\uc81c \uc601\uc0c1\uc744 \ucc3e\uc544 \uc918.');}catch(e){notice(e.message,true);}};
$('scan').onclick=async()=>{try{$('scan').disabled=true;const s=await message({kind:'scan'});render(s);notice(s.groups.length?'\uc601\uc0c1 \ud6c4\ubcf4\ub97c \ucc3e\uc558\uc5b4.':'\uc544\uc9c1 \uc8fc\uc18c\ub97c \ubabb \ucc3e\uc558\uc5b4. \uc601\uc0c1\uc744 \uc7ac\uc0dd\ud55c \ub4a4 \ud655\uc7a5\uc744 \ub2e4\uc2dc \uc5f4\uc5b4 \ucc3e\uc544 \uc918.');}catch(e){notice(e.message,true);}finally{$('scan').disabled=false;}};
message({kind:'status'}).then(render).catch(e=>notice(e.message,true));
