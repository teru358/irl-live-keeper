import {setCompletionRules} from './template-lexer.js';
import {mountEditor} from './editor.js';
import {sourceTemplates,templateBody,fileAccept} from './source-body.js';
import {updateRendered} from './reconcile.js';
import {html,render} from '../vendor/lit-html-3.3.3/lit-html.js';
import {live} from '../vendor/lit-html-3.3.3/directives/live.js';
import {repeat} from '../vendor/lit-html-3.3.3/directives/repeat.js';
import {createStore} from './store.js';
import {networkError} from './api.js';
import {createFilesRefresher,layout,remember,dirty,previewRequest,sourceURL,valueText,conditionText,applyPreview,applySourceEvent,deletePrompt,clearEditor,visibleLayout,templateDiagnostics,applyCollection,openEditor,testAxes,testInitial,testChoiceKey,simulateOf,testActive,testSignature,brokenError,sourceValueGroups,streamingChanged,testNoticeFor} from './sources-state.js';
export function mountSources(t,request,lang){
 const root=document.querySelector('#sources-panel');
 const mobile=window.matchMedia('(max-width:700px)');
 let storage;try{storage=window.localStorage;}catch(_){}
 const store=createStore({list:[],broken:{},diagnostics:[],names:[],defaults:null,draft:null,base:null,fresh:false,mobile:mobile.matches,layout:layout(storage),...testInitial(),texts:{},preview:null,files:[],fileNotices:[],notice:'',failNetwork:false,editorNotice:'',editorError:'',previewError:'',warnings:[],testNotice:'',testReason:'',busy:false,streaming:false,test:false,active:false,valueGroups:[],valuesOpen:false,valuesFilter:'',valuesError:'',settings:null,autoHost:''});
 const update=f=>store.update(f);let editor=null,editorHost=null,mounting=null,generation=0,previewVersion=0,timer,events,polling=false,valuesPolling=false,valuesVersion=0;
 const fail=e=>update(s=>{if(s.draft)s.editorError=e.message;else s.notice=e.message;s.failNetwork=!!e.network;});
 const refreshFiles=createFilesRefresher({request,update,fail});
 const sim=simulateOf;
 const frames=new WeakMap(), activeFrames=new Set();
 function cleanupFrames(){for(const c of activeFrames)if(!c.host.isConnected){c.observer.disconnect();activeFrames.delete(c);}}
 const rulesPromise=fetch('/source/assets/html_rules.json').then(r=>r.json()).then(r=>{setCompletionRules(r);return r;});
 function previewFrame(host,text='',name,width=1920,height=1080){
  let c=frames.get(host);
  if(!c){
   const frame=document.createElement('iframe');
   frame.setAttribute('sandbox','allow-same-origin');frame.src='/api/sources/frame';frame.title=t('src_preview_frame');
   c={host,frame,box:null,text,name,width,height};frames.set(host,c);activeFrames.add(c);
   const paint=()=>{
    const scale=Math.min(host.clientWidth/c.width,host.clientHeight/c.height,1);
    frame.style.width=c.width+'px';frame.style.height=c.height+'px';frame.style.transform='scale('+scale+')';
   };
   c.paint=paint;
   frame.addEventListener('load',()=>{c.box=frame.contentDocument.querySelector('.irl-source');refresh();});
   const observer=new ResizeObserver(paint);observer.observe(host);c.observer=observer;
   host.append(frame);
  }
  Object.assign(c,{text,name,width,height});c.paint();
  function refresh(){rulesPromise.then(rules=>{
   if(!host.isConnected){c.observer.disconnect();return;}
   if(c.box&&(c.lastName!==c.name||c.lastText!==c.text)){c.box.className='irl-source irl-source-'+c.name;if(updateRendered(c.box,c.text,rules,new DOMParser())){c.lastName=c.name;c.lastText=c.text;}}
  }).catch(console.error);}
  refresh();
 }
 async function load(){try{const d=await request('sources');const f=await request('source-files');const settings=await request('settings');const sender=await request('sender');update(s=>{applyCollection(s,d);s.files=f.files;s.fileNotices=f.notices||[];s.settings=settings.values;s.autoHost=sender.addresses.find(a=>a.kind==='lan')?.address||'';});await preview();}catch(e){fail(e);}}
 async function preview(){
  const version=++previewVersion,s=store.get();
  try{
   const data=await request('sources/preview',previewRequest(s.draft,s.list.map(x=>x.name),sim(s)));
   if(version!==previewVersion)return;
   update(x=>applyPreview(x,data));
  }catch(e){if(version!==previewVersion)return;if(s.draft)update(x=>{x.previewError=e.message;});else fail(e);}
 }
 async function refreshValues(){
  const s=store.get();
  if(!s.active||!s.draft||!s.valuesOpen||valuesPolling)return;
  const version=valuesVersion;valuesPolling=true;
  try{
   const data=await request('source-values',testActive(s)?{simulate:sim(s)}:undefined);
   if(version!==valuesVersion)return;
   update(x=>{x.valueGroups=data.groups;x.valuesError='';});
  }catch(e){if(version===valuesVersion)update(x=>{x.valuesError=e.message;});}
  finally{valuesPolling=false;if(version!==valuesVersion)refreshValues();}
 }
 function toggleValues(e){
  const open=e.currentTarget.open;
  if(store.get().valuesOpen===open)return;
  ++valuesVersion;update(s=>{s.valuesOpen=open;});
  if(open)refreshValues();
 }
 function change(fn){const before=store.get();++previewVersion;update(s=>{s.diagnostics=[];s.previewError='';s.editorNotice='';s.editorError='';fn(s);});const next=store.get();if(testSignature(before)!==testSignature(next)){++valuesVersion;refreshValues();}clearTimeout(timer);timer=setTimeout(()=>{timer=null;preview();},300);}
 function close(){const s=store.get();if(s.draft&&dirty(s.draft,s.base)&&!confirm(t('src_discard')))return false;++generation;++previewVersion;++valuesVersion;editor?.destroy();editor=null;editorHost=null;clearTimeout(timer);timer=null;update(clearEditor);return true;}
 async function open(src){if(!close())return;update(s=>{s.notice='';openEditor(s,src);if(!src){s.draft.body=templateBody(0,s.settings?.cameras);s.base=structuredClone(s.draft);}});preview();refreshFiles();}
 async function save(){const s=store.get(),snapshot=structuredClone(s.draft);update(x=>{x.busy=true;x.editorError='';x.editorNotice='';});try{await request('sources'+(s.fresh?'':'/'+encodeURIComponent(snapshot.name)),snapshot,s.fresh?'POST':'PUT');update(x=>{x.base=snapshot;x.fresh=false;x.editorNotice=t('src_saved');});await load();}catch(e){fail(e);}finally{update(x=>{x.busy=false;});}}
 async function remove(src){if(!confirm(deletePrompt(src,store.get().list,t)))return;update(s=>{s.notice='';});try{await request('sources/'+encodeURIComponent(src.name),undefined,'DELETE');await load();}catch(e){fail(e);}}
 async function obs(enabled){update(x=>{x.testNotice='';});try{const d=await request('sources/test-obs',enabled?sim(store.get()):undefined,enabled?'PUT':'DELETE');receive(d);}catch(e){fail(e);}}
 function receive(d){
  let reset=false;update(s=>{reset=applySourceEvent(s,d,t('src_stopped'));});
  if(reset){++valuesVersion;refreshValues();++previewVersion;clearTimeout(timer);timer=null;if(store.get().draft)preview();}
 }
 async function removeFile(file){
  if(!confirm(deletePrompt(file,store.get().list,t)))return;
  update(s=>{s.notice='';});
  const url=file.name.split('/').map(encodeURIComponent).join('/');
  try{await request('source-files/'+url+(file.used_by?.length?'?confirm=true':''),undefined,'DELETE');await load();}
  catch(e){
   // Sources started to use it after the list was read: ask again with the sources now using it.
   if(e.code==='source_files.in_use'&&!file.used_by?.length){
    if(confirm(deletePrompt({...file,used_by:e.used_by},store.get().list,t))){try{await request('source-files/'+url+'?confirm=true',undefined,'DELETE');await load();}catch(e2){fail(e2);}}
    return;
   }
   fail(e);
  }
 }
 function insertBody(text){editor?.insert(text);}
 async function uploadFiles(files){
  const version=generation;
  for(const file of files)try{
   const body=new FormData();body.append('file',file,file.name.normalize('NFC'));
   let r;try{r=await fetch('/api/source-files?lang='+lang,{method:'POST',headers:{'X-IRL-Live-Keeper':'1'},body});}catch(e){throw e instanceof TypeError?networkError(t):e;}
   const d=await r.json();if(!r.ok)throw Error(d.error||t('failed'));
   await load();
   if(version===generation)editor?.insertAsset(d.name);
  }catch(e){fail(e);}
 }
 async function upload(e){const input=e.target;await uploadFiles([...input.files]);input.value='';}
 function dragover(e){if(e.dataTransfer?.types.includes('Files'))e.preventDefault();}
 function drop(e){if(!e.dataTransfer?.files.length)return;e.preventDefault();e.stopPropagation();uploadFiles([...e.dataTransfer.files]);}
 async function syncEditor(s){
  const host=root.querySelector('#source-body');
  if(!host)return;
  if(editorHost!==host){
   if(mounting===host)return;
   mounting=host;const version=generation;
   try{
    const instance=await mountEditor(host,{value:s.draft.body,names:s.names,onFiles:uploadFiles,files:()=>store.get().files,label:t('src_body'),t,onChange:value=>change(x=>{if(x.draft)x.draft.body=value;})});
    if(version!==generation||!host.isConnected){instance.destroy();return;}
    editor=instance;editorHost=host;
   }catch(e){fail(e);}finally{if(mounting===host)mounting=null;}
  }
  editor?.setErrors(store.get().diagnostics);
 }

 mobile.addEventListener('change',e=>update(s=>{s.mobile=e.matches;}));
 const field=(s,key,type='text')=>{
  const value=s.draft[key]??'';
  const input=html`<input name=${'source-'+key} aria-label=${t('src_'+key)} type=${type} step="any" .value=${live(String(value))} ?disabled=${s.busy} @input=${e=>{
   const raw=e.target.value,v=type==='number'?(raw===''?0:Number(raw)):raw;
   change(x=>{x.draft[key]=v;});
  }}>`;
  const unit=['obs_width','obs_height'].includes(key)?'px':'';
  return html`<label class="source-field">${t('src_'+key)}<span class="source-input">${input}${unit?html`<span class="hint">${unit}</span>`:''}</span></label>`;
 };
 const segments=(label,values,current,select,name=v=>t('src_'+v))=>html`<div class="source-control"><span class="hint">${label}</span><div class="seg" role="group" aria-label=${label}>${values.map(v=>html`<button type="button" aria-pressed=${String(current===v)} @click=${()=>select(v)}>${name(v)}</button>`)}</div></div>`;
 // 軸や自由を変えたとき、OBS に反映中なら、実際だけに戻っていれば止め、そうでなければ新しい状況を送る。
 const retest=()=>{if(store.get().test)queueMicrotask(()=>obs(testActive(store.get())));};
 const choose=(axis,value)=>{change(x=>{x[axis]=value;x.testNotice='';});retest();};
 const testBar=s=>html`<div class="source-test" role="group" aria-label=${t('src_test')}>${Object.entries(testAxes).map(([axis,values])=>segments(t('src_'+axis),values,s[axis],value=>choose(axis,value),value=>t(testChoiceKey(axis,value))))}<div class="settings-field source-free"><label><input name="source-test-free" type="checkbox" role="switch" .checked=${s.free} @change=${e=>{const free=e.target.checked;change(x=>{x.free=free;x.testNotice='';});retest();}}>${t('src_custom')}</label></div>${s.free?html`<label class="source-field source-free-values">${t('src_custom_hint')}<textarea name="source-test-custom" .value=${live(s.custom)} @input=${e=>{const value=e.target.value;change(x=>{x.custom=value;});}}></textarea></label>`:''}<div class="settings-field source-obs"><label><input name="source-test-obs" type="checkbox" role="switch" .checked=${s.test} ?disabled=${s.streaming||(!s.test&&!testActive(s))} @change=${e=>obs(e.target.checked)}>${t('src_obs')}</label></div></div>`;
 const copy=(src,s)=>{
  const urls=sourceURL(s.settings,s.autoHost,src.name);
  const row=(url,kind)=>html`<label class="source-field">${t('src_url_'+kind)}<span class="source-url"><input name=${'source-url-'+kind+'-'+src.name} readonly .value=${url} aria-label=${t('src_url_'+kind)}><button class="btn" @click=${async()=>{try{await navigator.clipboard.writeText(url);}catch(_){fail(Error(t('sender_manual')));}}}>${t('sender_copy')}</button></span></label>`;
  return html`${row(urls.url,'local')}${urls.remoteURL?row(urls.remoteURL,'remote'):''}`;
 };
 const valuesList=s=>{
  const groups=sourceValueGroups(s.valueGroups,s.valuesFilter,t);
  return html`<details class="source-section source-values" .open=${s.valuesOpen} @toggle=${toggleValues}>
   <summary>${t('src_values')}${testActive(s)||s.free?html` <span class="source-values-test">${t('src_values_test')}</span>`:''}</summary>
   <label class="source-field">${t('src_values_filter')}<input name="source-values-filter" type="search" placeholder=${t('src_values_filter_hint')} .value=${live(s.valuesFilter)} @input=${e=>{const value=e.target.value;update(x=>{x.valuesFilter=value;});}}></label>
   <p role="alert">${s.valuesError}</p>
   <div class="source-values-scroll">${repeat(groups,g=>g.key,g=>html`<section class="source-value-group"><h3>${g.title}</h3>${repeat(g.values,v=>v.name,v=>html`<button type="button" class="source-value-row" aria-label=${v.label} @click=${()=>insertBody(valueText(v.name))}><span class="source-value-name">${v.name}</span><span>${v.meaning}</span><span class="source-value-text">${v.text}</span></button>`)}</section>`)}${!groups.length?html`<p class="hint">${t('src_values_empty')}</p>`:''}</div>
  </details>`;
 };
 function view(s){const {axis,showAxis}=visibleLayout(s.layout,s.mobile);return html`<h2>${t('sources')}</h2>${s.draft?html`
 <div class="source-header">${s.fresh?html`<label class="source-field">${t('src_template')}<select name="source-template" @change=${e=>{const body=templateBody(Number(e.target.value),s.settings?.cameras);editor?.setValue(body);if(!editor)change(x=>{x.draft.body=body;});}}>${sourceTemplates.map((v,i)=>html`<option value=${i}>${t('src_template_'+v.key)}</option>`)}</select></label>`:''}${field(s,'title')}${s.fresh?field(s,'name'):''}<div class="source-actions"><button class="btn primary" ?disabled=${s.busy} @click=${save}>${t('src_save')}</button><button class="btn" ?disabled=${s.busy} @click=${close}>${t('src_close')}</button>${dirty(s.draft,s.base)?html`<span class="source-dirty">${t('src_unsaved')}</span>`:''}</div></div>
 <p class="source-editor-notice" role="status">${s.editorNotice}</p><p role="alert">${s.editorError||s.previewError}</p>${s.warnings.map(w=>html`<p class="source-warning" role="status">${w.message}</p>`)}
 <div class="source-toolbar">${testBar(s)}${segments(t('src_view'),['editonly','previewonly','split'],s.layout.mode,mode=>setLayout({mode}))}${showAxis?segments(t('src_split_axis'),['horizontal','vertical'],axis,axis=>setLayout({axis})):''}</div>
 <div class=${'source-workspace '+s.layout.mode+' '+axis}><div class="source-edit" @dragover=${dragover} @drop=${drop}><div class="source-insert"><button class="btn" @click=${()=>insertBody(conditionText('cam1.kbps < 800'))}>${t('src_condition')}</button><label class="source-field">${t('src_files')}<select name="source-insert-file" @change=${e=>{if(e.target.value)editor?.insertAsset(e.target.value);e.target.value='';}}><option value="">${t('src_insert')}</option>${s.files.filter(f=>f.usable).map(f=>html`<option value=${f.name}>${f.name}</option>`)}</select></label></div>${valuesList(s)}<span class="source-editor-label" id="source-body-label">${t('src_body')}</span><div id="source-body" class="source-body" role="group" aria-labelledby="source-body-label"></div></div><div class="source-preview" data-preview="editor" aria-label=${t('src_preview_frame')}></div></div>
 <details class="source-section" @toggle=${e=>{if(e.currentTarget.open)refreshFiles();}} @dragover=${dragover} @drop=${drop}><summary>${t('src_files')}</summary><label class="source-field">${t('src_upload')}<input name="source-upload" type="file" accept=${fileAccept} @change=${upload}></label><p>${t('src_upload_names')}</p>${s.fileNotices.map(n=>html`<p role="status">${n.message}</p>`)}${s.files.map(f=>f.usable?html`<div class="source-file"><bdi>${f.name}</bdi><button class="btn" @click=${()=>editor?.insertAsset(f.name)}>${t('src_insert')}</button><button class="btn" @click=${()=>removeFile(f)}>${t('src_delete')}</button></div>`:html`<div class="source-file unusable"><bdi>${f.name}</bdi><span class="source-file-reason">${f.reason_text}</span>${f.reason==='name_collision'?html`<button class="btn" @click=${()=>removeFile(f)}>${t('src_delete')}</button>`:''}</div>`)}</details>
 <details class="source-section"><summary>${t('src_obs_import')}</summary><div class="source-fields">${field(s,'obs_width','number')}${field(s,'obs_height','number')}</div>${s.fresh?html`<p class="hint">${t('src_save_url')}</p>`:copy(s.draft,s)}<p>${t('src_steps')}</p>${sourceURL(s.settings,s.autoHost,s.draft.name).hint?html`<p class="hint">${t('src_lan_hint')}</p>`:''}</details>`:html`
 <div class="source-toolbar">${testBar(s)}<button class="btn primary" @click=${()=>open()}>${t('src_new')}</button></div><div class="source-cards">${repeat(s.list,src=>src.name,src=>html`<article><h3>${src.title||src.name}</h3>${brokenError(s.broken,src.name)?html`<div class="source-repair"><span class="source-repair-badge">${t('src_needs_repair')}</span><p>${brokenError(s.broken,src.name).error}</p></div>`:''}${copy(src,s)}<div class="source-preview" data-preview=${src.name}></div><div class="source-actions"><button class="btn" @click=${()=>open(src)}>${t('src_edit')}</button><button class="btn" @click=${()=>remove(src)}>${t('src_delete')}</button></div><details><summary>${t('src_obs_import')}</summary><p>${t('src_steps')}</p>${sourceURL(s.settings,s.autoHost,src.name).hint?html`<p>${t('src_lan_hint')}</p>`:''}<p>${t('src_width')}: ${src.obs_width} × ${t('src_height')}: ${src.obs_height}</p></details></article>`)}</div>`}<p class="hint">${t('src_font_hint')}</p><p role="status">${testNoticeFor(s)}</p>${s.draft?'':html`<p role="status">${s.notice}</p>`}`;}
 function setLayout(v){if(v.axis==='horizontal'&&store.get().mobile)return;const next={...store.get().layout,...v};remember(storage,next);update(s=>{s.layout=next;});}
 store.subscribe(s=>{try{render(view(s),root);syncEditor(s);cleanupFrames();for(const host of root.querySelectorAll('[data-preview]')){const key=host.dataset.preview;if(s.draft){previewFrame(host,s.preview?.text||'',s.draft.name,s.draft.obs_width,s.draft.obs_height);}else {const src=s.list.find(x=>x.name===key);previewFrame(host,brokenError(s.broken,key)?'':s.texts[key],key,src.obs_width,src.obs_height);};}}catch(e){console.error(e);render(html`<p role="alert">${t('failed')}</p>`,root);cleanupFrames();}});
 window.addEventListener('keeper-before-page',e=>{if(e.detail!=='sources'&&!close())e.preventDefault();});
 window.addEventListener('beforeunload',e=>{const s=store.get();if(s.draft&&dirty(s.draft,s.base)){e.preventDefault();e.returnValue='';}});
 window.addEventListener('keeper-status',e=>update(s=>streamingChanged(s,e.detail.obs.streaming)));window.addEventListener('keeper-reconnected',()=>update(s=>{if(s.failNetwork){s.notice='';s.editorError='';s.failNetwork=false;}}));
 window.addEventListener('keeper-page',e=>{const active=e.detail==='sources';++valuesVersion;update(s=>{s.active=active;if(!active){s.notice='';s.editorNotice='';s.testNotice='';}});events?.close();events=null;if(active){load();events=new EventSource('/api/events?topics=sources&lang='+lang);events.addEventListener('sources',e=>{try{receive(JSON.parse(e.data));}catch(err){console.error(err);}});}});
 setInterval(()=>{refreshValues();},1000);
 setInterval(async()=>{const s=store.get();if(!s.active||polling||timer)return;polling=true;try{if(testActive(s)||s.draft)await preview();}finally{polling=false;}},1000);
 setInterval(()=>{if(store.get().test)request('sources/test-obs/alive',{}).then(receive).catch(fail);},15000);
}
