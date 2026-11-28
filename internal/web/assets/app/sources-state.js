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
// A plain textarea: the same cursor rule as the editor, as a replace range plus the new cursor position.
export function textareaInsertion(value,selection,hasCursor,text){
 const {from,to,insert}=editorInsertion(value,selection,hasCursor,text);
 return {from,to,insert,cursor:from+insert.length};
}
// 回線の状況のテスト: 3つの軸（回線・待機・配信。どれも実際を持つ）と、軸とは別の「自由」。
export const testAxes={line:['actual','OK','LOW','DOWN'],standby:['actual','none','auto'],stream:['actual','streaming','offline']};
export const testInitial=()=>({line:'actual',standby:'actual',stream:'actual',free:false,custom:'{}'});
export const testChoiceKey=(axis,value)=>value==='actual'?'src_actual':'src_'+axis+'_'+value;
// 自由の JSON が、値の連想配列として読めればそれを、読めなければ null を返す。
export function freeValues(text){try{const v=JSON.parse(text);return v&&typeof v==='object'&&!Array.isArray(v)?v:null;}catch(_){return null;}}
const axisOf=(state,axis)=>state[axis]||'actual';
// サーバーの simulate の形。実際の軸は省く。自由がオンなら values を足す（読めない JSON は例外）。
export function simulateOf(state){
 const sim={};
 for(const axis of Object.keys(testAxes))if(axisOf(state,axis)!=='actual')sim[axis]=state[axis];
 if(state.free)sim.values=JSON.parse(state.custom);
 return sim;
}
// テスト中か（どれかの軸が実際でないか、自由に値がある）。「OBS にも反映」を使える条件も同じ。
export const testActive=state=>Object.keys(testAxes).some(axis=>axisOf(state,axis)!=='actual')||(!!state.free&&Object.keys(freeValues(state.custom)||{}).length>0);
export const testSignature=state=>JSON.stringify([state.line,state.standby,state.stream,state.free,state.custom]);
// 配信の開始で、すべての軸を実際に戻し、自由も切る。
export function testReset(state){state.line='actual';state.standby='actual';state.stream='actual';state.free=false;}
export function applyPreview(state,data){
 state.previewError=data.template_error?.error||'';
 state.warnings=data.template_error?[]:(data.warnings||[]);
 state.diagnostics=templateDiagnostics(data.template_error);
 if(state.draft){if(!data.template_error)state.preview=data;}
 else if(!data.template_error)state.texts=data.sources;
}
export function applySourceEvent(state,data,stopped){
 const test=data.test_obs;
 const reset=!test.enabled&&test.reason==='stream_started'&&(state.test||state.testReason!=='stream_started');
 if(reset){testReset(state);state.testNotice=stopped;if(state.preview)state.preview={...state.preview,text:''};}
 if(!testActive(state))state.texts=data.sources;
 state.test=test.enabled;state.testReason=test.reason;
 return reset;
}

// Deleting an asset that sources use: the titles, and that they will need repair.
export function deletePrompt(item,list,t){
 const label=source=>source.title||source.name;
 const prompt=t('src_confirmdelete').replace('{name}',()=>label(item));
 const used=item.used_by?.map(name=>label(list.find(src=>src.name===name)||{name}));
 return prompt+(used?.length?'\n'+t('src_used').replace('{sources}',()=>used.join(', '))+'\n'+t('src_file_used_note'):'');
}
// The list rows carry the camera ids of the body; a saved source does not, so the draft drops them.
export const sourceDraft=source=>{const {cameras,...draft}=source;return structuredClone(draft);};
export function clearEditor(state){
 state.draft=null;state.base=null;state.preview=null;state.warnings=[];
 state.valueGroups=[];state.valuesOpen=false;state.valuesFilter='';state.valuesError='';
 state.previewError='';state.editorError='';state.editorNotice='';state.diagnostics=[];
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
 state.draft=source?sourceDraft(source):structuredClone({...state.defaults,name:'',title:'',body:''});
 state.warnings=[];
 state.base=structuredClone(state.draft);state.fresh=!source;
 state.preview=null;state.editorNotice='';state.editorError='';
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
