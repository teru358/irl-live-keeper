import {cameraHeading} from './camera-name.js';
export function layout(storage){try{const v=JSON.parse(storage.getItem('source-layout'));return {mode:['editonly','previewonly','split'].includes(v?.mode)?v.mode:'split',axis:v?.axis==='vertical'?'vertical':'horizontal'};}catch(_){return {mode:'split',axis:'horizontal'};}}
export function remember(storage,value){try{storage.setItem('source-layout',JSON.stringify(value));}catch(_){}}
export const dirty=(draft,base)=>JSON.stringify(draft)!==JSON.stringify(base);
export const assetText=name=>'url("/source/files/'+encodeURIComponent(name)+'")';
export const previewRequest=(draft,names,simulate)=>({...draft?{source:draft}:{names},simulate});
export const valueText=name=>'{'+name+'}';
export const conditionText=(condition,body='')=>'{if '+condition+'}'+body+'{end}';

export function sourceURL(settings,auto,name){
 const admin=settings?.admin||{},host=(settings?.stream?.obs_read_host||'').replace(/^\[|\]$/g,'');
 const loop=/^127\./.test(host)||['localhost','::1'].includes(host.toLowerCase());
 const remote=host&&!loop?host:auto;
 const url=h=>'http://'+(h.includes(':')?'['+h.replace(/^\[|\]$/g,'')+']':h)+':'+(admin.port||8787)+'/source/'+encodeURIComponent(name);
 return {url:url('127.0.0.1'),remoteURL:admin.lan&&remote?url(remote):null,hint:!admin.lan&&!!host&&!loop};
}
export function editorInsertion(doc,selection,hasCursor,text,asset=false){
 const from=hasCursor?selection.from:doc.length,to=hasCursor?selection.to:doc.length;
 const insert=asset?(hasCursor?assetText(text):(doc&&!doc.endsWith('\n')?'\n':'')+'.irl-source { background-image: '+assetText(text)+'; }'):text;
 return {from,to,insert};
}
export const decorationInput=value=>value===0||value==null?'':String(value);
export const decorationNumber=value=>value===''?0:Number(value);
export function appearanceChanged(draft,base){
 const appearance=source=>Object.fromEntries(Object.entries(source).filter(([key])=>!['name','title','template','obs_width','obs_height'].includes(key)));
 return dirty(appearance(draft),appearance(base));
}
export function applyPreview(state,data){
 state.previewError=data.template_error?.error||'';
 state.diagnostics=templateDiagnostics(data.template_error);
 if(state.draft){if(!data.template_error)state.preview=data;}
 else if(!data.template_error)state.texts=data.sources;
}
export function applySourceEvent(state,data,stopped){
 const test=data.test_obs;
 const reset=!test.enabled&&test.reason==='stream_started'&&(state.test||state.testReason!=='stream_started');
 if(reset){state.mode='actual';state.testNotice=stopped;if(state.preview)state.preview={...state.preview,text:''};}
 if(state.mode==='actual')state.texts=data.sources;
 state.test=test.enabled;state.testReason=test.reason;
 return reset;
}
export function previewChanges(previous,next){
 return {text:!previous||previous.text!==next.text||previous.name!==next.name,
  style:!previous||previous.name!==next.name||previous.css!==next.css||previous.version!==next.version};
}
export function deletePrompt(item,list,t){
 const label=source=>source.title||source.name;
 const prompt=t('src_confirmdelete').replace('{name}',()=>label(item));
 const used=item.used_by?.map(name=>label(list.find(src=>src.name===name)||{name}));
 return prompt+(used?.length?'\n'+t('src_used').replace('{sources}',()=>used.join(', ')):'');
}
export function clearEditor(state){
 state.draft=null;state.base=null;state.preview=null;
 state.valueGroups=[];state.valuesOpen=false;state.valuesFilter='';state.valuesError='';
 state.previewError='';state.editorError='';state.editorNotice='';state.styleInputs={};state.diagnostics=[];
}
export const visibleLayout=(saved,mobile)=>({axis:mobile?'vertical':saved.axis,showAxis:saved.mode==='split'&&!mobile});

export const brokenError=(broken,name)=>Object.hasOwn(broken,name)?broken[name]:null;
export const templateDiagnostics=error=>error?[{line:error.line,column:error.column,message:error.error}]:[];
export function applyCollection(state,data){
 state.list=data.sources;state.broken=data.broken||{};
 state.names=data.names;state.defaults=data.default;
 for(const name of Object.keys(state.broken))state.texts[name]='';
 if(data.error)state.notice=data.error;
}
export function openEditor(state,source){
 state.draft=structuredClone(source||{...state.defaults,name:'',title:'',template:'',css:'',style:{}});
 state.base=structuredClone(state.draft);state.fresh=!source;
 state.preview=null;state.editorNotice='';state.editorError='';state.styleInputs={};
 const error=source?brokenError(state.broken,source.name):null;
 state.previewError=error?.error||'';state.diagnostics=templateDiagnostics(error);
}

// Labels use src_value_<definition name>; camera IDs share cam<n>.* labels.
export function sourceValueGroups(groups,filter,t){
 const query=filter.toLowerCase();
 return groups.map(group=>{
  const camera=group.kind==='camera'?group.camera:null;
  const key=camera?'camera:'+camera.id:group.kind;
  const title=camera?cameraHeading(camera,t):t('src_values_'+group.kind);
  const values=group.values.map(value=>{
   const name=camera?'cam<n>.'+value.name.slice((camera.id+'.').length):value.name;
   const meaning=t('src_value_'+name);
   return {...value,meaning,label:value.name+': '+meaning,text:value.text??'—'};
  }).filter(value=>value.name.toLowerCase().includes(query)||value.meaning.toLowerCase().includes(query));
  return {key,title,values};
 }).filter(group=>group.values.length);
}
