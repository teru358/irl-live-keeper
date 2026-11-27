import {html,render} from '../vendor/lit-html-3.3.3/lit-html.js';
import {live} from '../vendor/lit-html-3.3.3/directives/live.js';
import {repeat} from '../vendor/lit-html-3.3.3/directives/repeat.js';
import {createStore} from './store.js';
import {mountEditor} from './editor.js';
import {layout,remember,dirty,previewRequest,sourceURL,decorationInput,decorationNumber,appearanceChanged,applyPreview,applySourceEvent,previewChanges,deletePrompt,clearEditor,visibleLayout,templateDiagnostics,applyCollection,openEditor,brokenError,sourceValueGroups} from './sources-state.js';
export function mountSources(t,request,lang){
 const root=document.querySelector('#sources-panel');
 const mobile=window.matchMedia('(max-width:700px)');
 let storage;try{storage=window.localStorage;}catch(_){}
 const store=createStore({list:[],broken:{},diagnostics:[],names:[],defaults:null,draft:null,base:null,fresh:false,mobile:mobile.matches,layout:layout(storage),mode:'actual',custom:'{}',texts:{},preview:null,files:[],notice:'',editorNotice:'',editorError:'',previewError:'',testNotice:'',testReason:'',styleInputs:{},styleVersions:{},busy:false,streaming:false,test:false,active:false,valueGroups:[],valuesOpen:false,valuesFilter:'',valuesError:'',settings:null,autoHost:''});
 const update=f=>store.update(f);let editors=[],generation=0,previewVersion=0,timer,events,polling=false,valuesPolling=false,valuesVersion=0;
 const fail=e=>update(s=>{if(s.draft)s.editorError=e.message;else s.notice=e.message;});
 const sim=s=>({mode:({ok:'OK',low:'LOW',down:'DOWN',auto:'standby'})[s.mode]||s.mode,...s.mode==='custom'?{values:JSON.parse(s.custom)}:{}});
 const sourcePath=n=>'/source/'+encodeURIComponent(n);
 // DOM component state lives with each host; renders only update changed parts.
 const shadows=new WeakMap();
 function shadow(host,text='',css,name,version=0){
  let component=shadows.get(host);
  const next={name,text,css,version},changes=previewChanges(component?.value,next);
  if(!component){
   const root=host.shadowRoot||host.attachShadow({mode:'open'});
   const box=document.createElement('div'),span=document.createElement('span');
   span.className='irl-text';box.append(span);root.append(box);
   component={root,box,span};shadows.set(host,component);
  }
  if(changes.style){
   if(css!==undefined){
    component.sheet??=new CSSStyleSheet();component.sheet.replaceSync(css);
    component.root.adoptedStyleSheets=[component.sheet];
   }else{
    if(!component.link){component.link=document.createElement('link');component.link.rel='stylesheet';component.root.prepend(component.link);}
    component.link.href=sourcePath(name)+'/style.css?v='+version;
   }
  }
  if(changes.text){
   component.box.className='irl-source irl-source-'+name+(text?'':' irl-empty');
   component.span.textContent=text;
  }
  component.value=next;
 }
 async function load(){try{const d=await request('sources');const f=await request('source-files');const settings=await request('settings');const sender=await request('sender');update(s=>{applyCollection(s,d);s.files=f.files;s.settings=settings.values;s.autoHost=sender.addresses.find(a=>a.kind==='lan')?.address||'';});await preview();}catch(e){fail(e);}}
 async function preview(){
  const version=++previewVersion,s=store.get();
  try{
   const data=await request('sources/preview',previewRequest(s.draft,s.list.map(x=>x.name),sim(s)));
   if(version!==previewVersion)return;
   editors[0]?.setErrors(templateDiagnostics(data.template_error));
   update(x=>applyPreview(x,data));
  }catch(e){if(version!==previewVersion)return;if(s.draft)update(x=>{x.previewError=e.message;});else fail(e);}
 }
 async function refreshValues(){
  const s=store.get();
  if(!s.active||!s.draft||!s.valuesOpen||valuesPolling)return;
  const version=valuesVersion;valuesPolling=true;
  try{
   const data=await request('source-values',s.mode==='actual'?undefined:{simulate:sim(s)});
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
 function change(fn){const before=store.get();++previewVersion;editors[0]?.setErrors([]);update(s=>{s.diagnostics=[];s.previewError='';fn(s);});const next=store.get();if(before.mode!==next.mode||before.custom!==next.custom){++valuesVersion;refreshValues();}clearTimeout(timer);timer=setTimeout(()=>{timer=null;preview();},300);}
 function close(){const s=store.get();if(s.draft&&dirty(s.draft,s.base)&&!confirm(t('src_discard')))return false;++generation;++previewVersion;++valuesVersion;for(const ed of editors)ed.destroy();editors=[];clearTimeout(timer);timer=null;update(clearEditor);return true;}
 async function open(src){if(!close())return;update(s=>openEditor(s,src));const token=++generation;await new Promise(queueMicrotask);try{const s=store.get();for(const kind of ['template','css']){const ed=await mountEditor(root.querySelector('[data-editor="'+kind+'"]'),{kind,value:s.draft[kind],names:s.names,label:t('src_'+kind),t,onChange:value=>change(x=>{x.draft[kind]=value;})});if(token!==generation){ed.destroy();return;}editors.push(ed);}editors[0]?.setErrors(store.get().diagnostics);preview();}catch(e){fail(e);}}
 async function save(){const s=store.get(),snapshot=structuredClone(s.draft);update(x=>{x.busy=true;x.editorError='';x.editorNotice='';});try{await request('sources'+(s.fresh?'':'/'+encodeURIComponent(snapshot.name)),snapshot,s.fresh?'POST':'PUT');update(x=>{x.base=snapshot;x.fresh=false;x.editorNotice=t('src_saved')+(!s.fresh&&appearanceChanged(snapshot,s.base)?'\n'+t('src_obs_refresh'):'');x.styleVersions[snapshot.name]=(x.styleVersions[snapshot.name]||0)+1;});await load();}catch(e){if(e.line){const diagnostics=templateDiagnostics({...e,error:e.message});editors[0]?.setErrors(diagnostics);update(x=>{x.previewError=e.message;x.diagnostics=diagnostics;});}else fail(e);}finally{update(x=>{x.busy=false;});}}
 async function remove(src){if(!confirm(deletePrompt(src,store.get().list,t)))return;try{await request('sources/'+encodeURIComponent(src.name),undefined,'DELETE');await load();}catch(e){fail(e);}}
 async function obs(enabled){update(x=>{x.testNotice='';});try{const d=await request('sources/test-obs',enabled?sim(store.get()):undefined,enabled?'PUT':'DELETE');receive(d);}catch(e){fail(e);}}
 function receive(d){
  let reset=false;update(s=>{reset=applySourceEvent(s,d,t('src_stopped'));});
  if(reset){++valuesVersion;refreshValues();++previewVersion;clearTimeout(timer);timer=null;if(store.get().draft)preview();}
 }
 async function removeFile(file){if(!confirm(deletePrompt(file,store.get().list,t)))return;try{await request('source-files/'+encodeURIComponent(file.name)+(file.used_by?.length?'?confirm=true':''),undefined,'DELETE');await load();}catch(e){fail(e);}}
 function insertCSSAsset(name){const editor=editors[1];if(!editor)return;editor.view.dom.closest('details').open=true;editor.insertAsset(name);}
 async function upload(e){const input=e.target,file=input.files[0];if(!file)return;try{const body=new FormData();body.append('file',file);const r=await fetch('/api/source-files?lang='+lang,{method:'POST',headers:{'X-IRL-Live-Keeper':'1'},body});const d=await r.json();if(!r.ok)throw Error(d.error||t('failed'));await load();}catch(e){fail(e);}finally{input.value='';}}

 mobile.addEventListener('change',e=>update(s=>{s.mobile=e.matches;}));
 const field=(s,key,type='text',style=false)=>{
  const stored=(style?s.draft.style[key]:s.draft[key])??'';
  const value=style?(s.styleInputs[key]??decorationInput(stored)):stored;
  const input=kind=>html`<input name=${'source-'+(style?'style-':'')+key+(kind==='color'?'-picker':'')} aria-label=${t('src_'+key)} type=${kind} step="any" .value=${live(String(kind==='color'?(/^#[0-9a-f]{6,8}$/i.test(value)?value.slice(0,7):'#000000'):value))} ?disabled=${s.busy} @input=${e=>{
   const raw=e.target.value,v=type==='number'?decorationNumber(raw):raw;
   change(x=>{(style?x.draft.style:x.draft)[key]=v;if(style)x.styleInputs[key]=raw;});
  }}>`;
  const unit=['font_size','outline_width','shadow_blur','shadow_x','shadow_y','border_radius','padding','obs_width','obs_height'].includes(key)?'px':key==='background_opacity'?'0–1':key==='font_weight'?'100–900':'';
  return html`<label class="source-field">${t('src_'+key)}<span class="source-input">${style&&key.includes('color')?html`<span class=${'source-color'+(value?'':' empty')} title=${value||t('src_unset')}>${input('color')}</span>`:''}${input(type)}${unit?html`<span class="hint">${unit}</span>`:''}</span></label>`;
 };
 const segments=(label,values,current,select)=>html`<div class="source-control"><span class="hint">${label}</span><div class="seg" role="group" aria-label=${label}>${values.map(v=>html`<button type="button" aria-pressed=${String(current===v)} @click=${()=>select(v)}>${t('src_'+v)}</button>`)}</div></div>`;
 const testBar=s=>html`<div class="source-test">${segments(t('src_test'),['actual','ok','low','down','auto','offline','custom'],s.mode,mode=>{change(x=>{x.mode=mode;x.testNotice='';});if(s.test)queueMicrotask(()=>obs(true));})}${s.mode==='custom'?html`<label class="source-field">${t('src_custom_hint')}<textarea name="source-test-custom" .value=${live(s.custom)} @input=${e=>{const value=e.target.value;change(x=>{x.custom=value;});}}></textarea></label>`:''}<div class="settings-field source-obs"><label><input name="source-test-obs" type="checkbox" role="switch" .checked=${s.test} ?disabled=${s.streaming} @change=${e=>obs(e.target.checked)}>${t('src_obs')}</label></div></div>`;
 const copy=(src,s)=>{
  const urls=sourceURL(s.settings,s.autoHost,src.name);
  const row=(url,kind)=>html`<label class="source-field">${t('src_url_'+kind)}<span class="source-url"><input name=${'source-url-'+kind+'-'+src.name} readonly .value=${url} aria-label=${t('src_url_'+kind)}><button class="btn" @click=${async()=>{try{await navigator.clipboard.writeText(url);}catch(_){fail(Error(t('sender_manual')));}}}>${t('sender_copy')}</button></span></label>`;
  return html`${row(urls.url,'local')}${urls.remoteURL?row(urls.remoteURL,'remote'):''}`;
 };
 const valuesList=s=>{
  const groups=sourceValueGroups(s.valueGroups,s.valuesFilter,t);
  return html`<details class="source-section source-values" .open=${s.valuesOpen} @toggle=${toggleValues}>
   <summary>${t('src_values')}${s.mode!=='actual'?html` <span class="source-values-test">${t('src_values_test')}</span>`:''}</summary>
   <label class="source-field">${t('src_values_filter')}<input name="source-values-filter" type="search" placeholder=${t('src_values_filter_hint')} .value=${live(s.valuesFilter)} @input=${e=>{const value=e.target.value;update(x=>{x.valuesFilter=value;});}}></label>
   <p role="alert">${s.valuesError}</p>
   <div class="source-values-scroll">${repeat(groups,g=>g.key,g=>html`<section class="source-value-group"><h3>${g.title}</h3>${repeat(g.values,v=>v.name,v=>html`<button type="button" class="source-value-row" aria-label=${v.label} @click=${()=>editors[0]?.insertValue(v.name)}><span class="source-value-name">${v.name}</span><span>${v.meaning}</span><span class="source-value-text">${v.text}</span></button>`)}</section>`)}${!groups.length?html`<p class="hint">${t('src_values_empty')}</p>`:''}</div>
  </details>`;
 };
 function view(s){const {axis,showAxis}=visibleLayout(s.layout,s.mobile);return html`<h2>${t('sources')}</h2>${s.draft?html`
 <div class="source-header">${field(s,'title')}${s.fresh?field(s,'name'):''}<div class="source-actions"><button class="btn primary" ?disabled=${s.busy} @click=${save}>${t('src_save')}</button><button class="btn" ?disabled=${s.busy} @click=${close}>${t('src_close')}</button>${dirty(s.draft,s.base)?html`<span class="source-dirty">${t('src_unsaved')}</span>`:''}</div></div>
 <p class="source-editor-notice" role="status">${s.editorNotice}</p><p role="alert">${s.editorError||s.previewError}</p>
 <div class="source-toolbar">${testBar(s)}${segments(t('src_view'),['editonly','previewonly','split'],s.layout.mode,mode=>setLayout({mode}))}${showAxis?segments(t('src_split_axis'),['horizontal','vertical'],axis,axis=>setLayout({axis})):''}</div>
 <div class=${'source-workspace '+s.layout.mode+' '+axis}><div class="source-edit"><div class="source-insert"><button class="btn" @click=${()=>editors[0]?.insertCondition('camera.kbps < 800','')}>${t('src_condition')}</button><label class="source-field">${t('src_files')}<select name="source-insert-file" @change=${e=>{if(e.target.value)editors[0]?.insert('/source/files/'+encodeURIComponent(e.target.value));e.target.value='';}}><option value="">${t('src_insert')}</option>${s.files.map(f=>html`<option value=${f.name}>${f.name}</option>`)}</select></label></div>${valuesList(s)}<div class="source-editor-label">${t('src_template')}</div><div data-editor="template"></div></div><div class="source-preview" data-preview="editor" aria-label=${t('src_previewonly')}></div></div>
 <details class="source-section"><summary>${t('src_decoration')}</summary><div class="source-fields">${['font','font_size','color','font_weight','outline_color','outline_width','shadow_color','shadow_blur','shadow_x','shadow_y','background_color','background_opacity','border_radius','padding'].map(k=>field(s,k,['font','color','outline_color','shadow_color','background_color'].includes(k)?'text':'number',true))}${['align','transition'].map(k=>html`<label class="source-field">${t('src_'+k)}<select name=${'source-style-'+k} .value=${s.draft.style[k]||''} @change=${e=>{const v=e.target.value;change(x=>{x.draft.style[k]=v;});}}><option value=""></option>${(k==='align'?['left','center','right']:['none','fade']).map(v=>html`<option value=${v} ?selected=${s.draft.style[k]===v}>${t('src_'+v)}</option>`)}</select></label>`)}</div></details>
 <details class="source-section"><summary>${t('src_css')}</summary><p class="hint">${t('src_css_hint')}</p><div data-editor="css"></div></details>
 <details class="source-section"><summary>${t('src_files')}</summary><label class="source-field">${t('src_upload')}<input name="source-upload" type="file" accept="image/png,image/jpeg,image/gif,image/webp" @change=${upload}></label>${s.files.map(f=>html`<div class="source-file"><span>${f.name}</span><button class="btn" @click=${()=>insertCSSAsset(f.name)}>${t('src_insert')}</button><button class="btn" @click=${()=>removeFile(f)}>${t('src_delete')}</button></div>`)}</details>
 <details class="source-section"><summary>${t('src_obs_import')}</summary><div class="source-fields">${field(s,'obs_width','number')}${field(s,'obs_height','number')}</div>${s.fresh?html`<p class="hint">${t('src_save_url')}</p>`:copy(s.draft,s)}<p>${t('src_steps')}</p>${sourceURL(s.settings,s.autoHost,s.draft.name).hint?html`<p class="hint">${t('src_lan_hint')}</p>`:''}</details>`:html`
 <div class="source-toolbar">${testBar(s)}<button class="btn primary" @click=${()=>open()}>${t('src_new')}</button></div><div class="source-cards">${repeat(s.list,src=>src.name,src=>html`<article><h3>${src.title||src.name}</h3>${brokenError(s.broken,src.name)?html`<div class="source-repair"><span class="source-repair-badge">${t('src_needs_repair')}</span><p>${brokenError(s.broken,src.name).error}</p></div>`:''}${copy(src,s)}<div class="source-preview" data-preview=${src.name}></div><div class="source-actions"><button class="btn" @click=${()=>open(src)}>${t('src_edit')}</button><button class="btn" @click=${()=>remove(src)}>${t('src_delete')}</button></div><details><summary>${t('src_obs_import')}</summary><p>${t('src_steps')}</p>${sourceURL(s.settings,s.autoHost,src.name).hint?html`<p>${t('src_lan_hint')}</p>`:''}<p>${t('src_width')}: ${src.obs_width} × ${t('src_height')}: ${src.obs_height}</p></details></article>`)}</div>`}<p role="status">${s.testNotice}</p>${s.draft?'':html`<p role="status">${s.notice}</p>`}`;}
 function setLayout(v){if(v.axis==='horizontal'&&store.get().mobile)return;const next={...store.get().layout,...v};remember(storage,next);update(s=>{s.layout=next;});}
 store.subscribe(s=>{try{render(view(s),root);for(const host of root.querySelectorAll('[data-preview]')){const key=host.dataset.preview;if(s.draft){shadow(host,s.preview?.text||'',s.preview?.css||'',s.draft.name);}else shadow(host,brokenError(s.broken,key)?'':s.texts[key],undefined,key,s.styleVersions[key]||0);}}catch(e){console.error(e);render(html`<p role="alert">${t('failed')}</p>`,root);}});
 window.addEventListener('keeper-before-page',e=>{if(e.detail!=='sources'&&!close())e.preventDefault();});
 window.addEventListener('beforeunload',e=>{const s=store.get();if(s.draft&&dirty(s.draft,s.base)){e.preventDefault();e.returnValue='';}});
 window.addEventListener('keeper-status',e=>update(s=>{s.streaming=e.detail.obs.streaming;}));
 window.addEventListener('keeper-page',e=>{const active=e.detail==='sources';++valuesVersion;update(s=>{s.active=active;});events?.close();events=null;if(active){load();events=new EventSource('/api/events?topics=sources&lang='+lang);events.addEventListener('sources',e=>{try{receive(JSON.parse(e.data));}catch(err){console.error(err);}});}});
 setInterval(()=>{refreshValues();},1000);
 setInterval(async()=>{const s=store.get();if(!s.active||polling||timer)return;polling=true;try{if(s.mode!=='actual'||s.draft)await preview();}finally{polling=false;}},1000);
 setInterval(()=>{if(store.get().test)request('sources/test-obs/alive',{}).then(receive).catch(fail);},15000);
}
