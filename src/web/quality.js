// Optional local quality workspace. All text from media/files uses textContent.
(() => {
 'use strict';
 const $=id=>document.getElementById(id);
 const make=(tag,text='',cls='')=>{const n=document.createElement(tag);n.textContent=text;if(cls)n.className=cls;return n;};
 const style=make('style');style.textContent=`.quality-panel{margin:14px 0;border:1px solid #45515f;border-radius:12px;background:#17212a;padding:14px}.quality-panel summary{cursor:pointer;font-weight:700;font-size:14px}.q-row{display:flex;align-items:center;flex-wrap:wrap;gap:9px;margin:13px 0}.quality-panel button,.quality-panel select,.quality-panel input{font:inherit;font-size:13px;padding:9px;border:1px solid #536471;border-radius:7px;background:#24313d;color:#edf2f5;max-width:100%}.quality-panel button{cursor:pointer}.quality-panel button:disabled{opacity:.45;cursor:default}.quality-panel label{font-size:12px;display:flex;gap:6px;align-items:center}.quality-panel input[type=number]{width:88px}.quality-panel .primary{background:#d9ff57;color:#192017;font-weight:800}.q-note{font-size:12px;color:#b5c2ce;line-height:1.6;margin:8px 0}.q-source{font-size:13px;overflow-wrap:anywhere}.q-status{white-space:pre-line;color:#d9ff57;font-size:12px;line-height:1.6}.q-status.error{color:#ffafb8}.q-comparison{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin:14px 0}.q-comparison figure{min-width:0;margin:0}.q-comparison img,.q-comparison video{width:100%;max-height:330px;object-fit:contain;background:#090e13;display:block;border-radius:7px}.q-comparison figcaption{font-size:11px;margin-bottom:7px;color:#b5c2ce}.quality-panel [hidden]{display:none!important}.quality-panel :focus-visible{outline:2px solid #d9ff57;outline-offset:2px}@media(max-width:600px){.q-comparison{grid-template-columns:1fr}.q-row{align-items:stretch}}`;
 document.head.append(style);
 const panel=make('details','','quality-panel');panel.id='qualityPanel';
 panel.append(make('summary','\ud654\uc9c8 \uac1c\uc120 \u00b7 \uc774\ubbf8\uc9c0 / \uc601\uc0c1 \ud655\ub300'));
 const choose=make('button','\ud30c\uc77c \uc120\ud0dd');choose.type='button';choose.id='qualityChoose';
 const sourceText=make('div','\uc6d0\ubcf8\uc740 \uadf8\ub300\ub85c, \uacb0\uacfc\ub294 \uc0ac\ubcf8\uc73c\ub85c \uc800\uc7a5\ud574.','q-source');sourceText.id='qualitySource';
 const row1=make('div','','q-row');row1.append(choose,sourceText);panel.append(row1);
 const select=(id,items)=>{const n=make('select');n.id=id;for(const [value,text]of items){const opt=make('option',text);opt.value=value;n.append(opt)}return n;};
 const mode=select('qualityMode',[['resize','\uc6d0\ud615 \uc720\uc9c0 \ud655\ub300 (\ube44AI)'],['ai','AI \ub514\ud14c\uc77c \ubcf4\uc815']]);
 const scale=select('qualityScale',[['2','2\ubc30'],['4','4\ubc30']]);
 const strength=select('qualityStrength',[['0.35','\uc57d\ud558\uac8c'],['0.65','\ud45c\uc900']]);
 const row2=make('div','','q-row');for(const [text,node]of [['\ubc29\uc2dd',mode],['\ud655\ub300',scale],['AI \uac15\ub3c4',strength]]){const label=make('label',text);label.append(node);row2.append(label)}panel.append(row2);
 const setup=make('button','AI \ub3c4\uad6c \uc900\ube44 (\ucd5c\ucd08 46MB)');setup.type='button';setup.id='qualityInstall';row2.append(setup);
 const segment=make('div','','q-row');segment.id='qualitySegment';segment.hidden=true;
 const number=(id,value)=>{const x=make('input');x.type='number';x.min='0';x.step='0.1';x.value=value;x.id=id;return x;};
 const start=number('qualityStart','0'),seconds=number('qualitySeconds','10');seconds.min='0.1';seconds.max='120';
 for(const [text,input]of [['\uc2dc\uc791 (\ucd08)',start],['\uae38\uc774 (\ucd08)',seconds]]){const label=make('label',text);label.append(input);segment.append(label)}
 segment.append(make('span','\uc120\ud0dd \uad6c\uac04\ub9cc \ucc98\ub9ac \u00b7 \ucd5c\ub300 120\ucd08','q-note'));panel.append(segment);
 const note=make('p','','q-note');note.id='qualityNote';panel.append(note);
 const row3=make('div','','q-row');
 const preview=make('button','\ube44\uad50\ubcf8 \ub9cc\ub4e4\uae30');preview.id='qualityPreview';preview.type='button';
 const save=make('button','\uc774 \uc124\uc815\uc73c\ub85c \uc0ac\ubcf8 \uc800\uc7a5','primary');save.id='qualitySave';save.type='button';save.disabled=true;row3.append(preview,save);panel.append(row3);
 const status=make('p','','q-status');status.id='qualityStatus';status.setAttribute('role','status');status.setAttribute('aria-live','polite');panel.append(status);
 const comparison=make('div','','q-comparison');comparison.id='qualityComparison';panel.append(comparison);
 $('premierePanel')?.after(panel);
 let source=null,approved='',previewID='',requestedKey='',busy=false,requesting=false,engineReady=false,setupID='',shownID='';
 const tell=(text,error=false)=>{status.textContent=text;status.classList.toggle('error',error);};
 const options=()=>({mode:mode.value,scale:Number(scale.value),strength:Number(strength.value),start:source?.kind==='video'?Number(start.value):0,seconds:source?.kind==='video'?Number(seconds.value):0});
 const key=()=>JSON.stringify([source?.id,options()]);
 const explain=()=>{
  strength.parentElement.hidden=mode.value!=='ai';setup.hidden=mode.value!=='ai'||engineReady;
  note.textContent=mode.value==='ai'?'AI\ub294 \uc5c6\ub358 \uc815\ubcf4\ub97c \ucd94\uc815\ud574. \ud53c\ubd80\u00b7\ud138\u00b7\uae00\uc790\uac00 \ub2ec\ub77c\uc9c0\uac70\ub098 \uc601\uc0c1\uc774 \uae5c\ube61\uc77c \uc218 \uc788\uc5b4. \uba3c\uc800 \ube44\uad50\ud558\uace0 \uc800\uc7a5\ud574.':'AI\ub97c \uc4f0\uc9c0 \uc54a\uace0 \ud06c\uae30\ub97c \ub298\ub824. \uc783\uc5b4\ubc84\ub9b0 \uc815\ubcf4\uac00 \ubcf5\uad6c\ub418\ub294 \uac83\uc740 \uc544\ub2c8\uc57c.';
  if(source)note.textContent+=' \uacb0\uacfc '+(source.width*Number(scale.value))+'\u00d7'+(source.height*Number(scale.value))+'px.';
  const wait=busy||requesting;choose.disabled=wait;setup.disabled=wait;preview.disabled=wait||!source||(mode.value==='ai'&&!engineReady);save.disabled=wait||!approved||requestedKey!==key();
  preview.textContent=source?.kind==='video'?'3\ucd08 \ube44\uad50\ubcf8 \ub9cc\ub4e4\uae30':'\ube44\uad50\ubcf8 \ub9cc\ub4e4\uae30';
 };
 const invalidate=()=>{approved='';shownID='';comparison.replaceChildren();explain();};
 [mode,scale,strength,start,seconds].forEach(x=>x.addEventListener('change',invalidate));
 const checkEngine=async()=>{try{const r=await fetch('/api/quality/status');if(r.ok)engineReady=!!(await r.json()).engine_ready;}catch(_){}explain();};
 choose.onclick=async()=>{requesting=true;explain();tell('\ud30c\uc77c \uc120\ud0dd\ucc3d\uc744 \ud655\uc778\ud574 \uc918.');try{
  const r=await post('/api/quality/choose',{});if(!r.cancelled){source=r;sourceText.textContent=r.name+' \u00b7 '+r.width+'\u00d7'+r.height+(r.kind==='video'?' \u00b7 '+r.duration.toFixed(1)+'\ucd08':'');segment.hidden=r.kind!=='video';seconds.value=String(Math.min(10,r.duration||10));start.value='0';invalidate();tell('\ubc30\uc728\uc744 \uace0\ub974\uace0 \ube44\uad50\ubcf8\uc744 \ub9cc\ub4e4\uc5b4 \ubd10.');}else tell('\uc120\ud0dd\uc744 \ucde8\uc18c\ud588\uc5b4.');
 }catch(e){tell(e.message,true);}finally{requesting=false;explain();}};
 setup.onclick=async()=>{if(!confirm('\uacf5\uc2dd Real-ESRGAN \ub3c4\uad6c\uc640 AI \ubaa8\ub378(\uc57d 46MB)\uc744 \uc774 PC\uc5d0 \ub2e4\uc6b4\ub85c\ub4dc\ud560\uae4c? \ud30c\uc77c\uc740 \uc678\ubd80\ub85c \uc804\uc1a1\ud558\uc9c0 \uc54a\uc544.'))return;requesting=true;explain();try{const r=await post('/api/quality/install',{consent:true});setupID=r.id;tell('AI \ub3c4\uad6c \uc124\uce58 \ub300\uae30');await poll();}catch(e){tell(e.message,true);}finally{requesting=false;explain();}};
 const enqueue=async(isPreview)=>{requesting=true;explain();try{
  const current=key();const r=await post('/api/quality/start',{source:source.id,options:options(),preview:isPreview,approved_preview:approved});
  if(isPreview){previewID=r.id;requestedKey=current;approved='';shownID='';comparison.replaceChildren();}
  tell(isPreview?'\ube44\uad50\ubcf8 \ucc98\ub9ac \ub300\uae30':'\uc0ac\ubcf8 \uc800\uc7a5 \ub300\uae30 \u00b7 \uc6d0\ubcf8\uc740 \ubcc0\uacbd\ud558\uc9c0 \uc54a\uc544.');await poll();
 }catch(e){tell(e.message,true);}finally{requesting=false;explain();}};
 preview.onclick=()=>enqueue(true);save.onclick=()=>enqueue(false);
 const show=j=>{
  if(shownID===j.id)return;shownID=j.id;comparison.replaceChildren();const videos=[];
  for(const [side,label]of [['before','\uc6d0\ubcf8 \ud655\ub300 (\ube44AI)'],['after','\uc120\ud0dd\ud55c \ubc29\uc2dd\uc758 \uacb0\uacfc']]){
   const figure=make('figure');figure.append(make('figcaption',label));const media=make(source.kind==='video'?'video':'img');media.src='/api/quality/preview/'+encodeURIComponent(j.id)+'/'+side;
   if(source.kind==='video'){media.controls=true;media.preload='metadata';media.playsInline=true;videos.push(media);}else media.alt=label;
   figure.append(media);comparison.append(figure);
  }
  if(videos.length===2){const [a,b]=videos;a.muted=true;b.onplay=()=>{a.currentTime=b.currentTime;a.play().catch(()=>{});};b.onpause=()=>a.pause();b.onseeked=()=>{a.currentTime=b.currentTime;};}
 };
 const oldRender=render;render=function(st){oldRender(st);const jobs=st?.jobs||[];busy=jobs.some(j=>(j.task==='quality'||j.task==='quality-engine')&&['queued','downloading'].includes(j.status));
  const j=jobs.find(j=>j.id===previewID);
  if(j&&requestedKey===key()){
   if(j.status==='done'){approved=j.id;show(j);tell('\ube44\uad50\ubcf8 \uc644\ub8cc. \uae00\uc790\u00b7\uc5bc\uad74\u00b7\uac00\uc7a5\uc790\ub9ac\ub97c \ud655\uc778\ud558\uace0 \uc800\uc7a5\ud574.');}
   else if(j.status==='error'||j.status==='cancelled'){approved='';tell(j.detail,true);}else tell(j.detail);
  }
  if(setupID){const engineJob=jobs.find(j=>j.id===setupID);if(engineJob?.status==='done'){setupID='';checkEngine();}else if(engineJob?.status==='error'){setupID='';tell(engineJob.detail,true);}}
  explain();
 };
 const oldJob=renderJob;renderJob=function(j){const card=oldJob(j);if(j.task==='quality'||j.task==='quality-engine'){
  card.querySelectorAll('.premiere-convert,.retry-btn').forEach(x=>x.remove());const url=card.querySelector('.job-url');if(url)url.textContent='PC \ud30c\uc77c \u00b7 \uc6d0\ubcf8 \ubcf4\uc874';
 }return card;};
 const oldSignature=jobSignature;jobSignature=jobs=>oldSignature(jobs)+JSON.stringify(jobs.map(j=>[j.quality?.preview,j.force_compatible]));
 lastRecentHistorySignature='';lastOlderHistorySignature='';checkEngine();if(lastState)render(lastState);else explain();
})();
