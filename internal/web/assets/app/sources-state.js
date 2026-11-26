export function layout(storage){try{const v=JSON.parse(storage.getItem('source-layout'));return {mode:['editonly','previewonly','split'].includes(v?.mode)?v.mode:'split',axis:v?.axis==='vertical'?'vertical':'horizontal'};}catch(_){return {mode:'split',axis:'horizontal'};}}
export function remember(storage,value){try{storage.setItem('source-layout',JSON.stringify(value));}catch(_){}}
export const dirty=(draft,base)=>JSON.stringify(draft)!==JSON.stringify(base);
export const assetText=name=>'url("/source/files/'+encodeURIComponent(name)+'")';
export const previewRequest=(draft,names,simulate)=>({...draft?{source:draft}:{names},simulate});
export const valueText=name=>'{'+name+'}';
export const conditionText=(condition,body='')=>'{if '+condition+'}'+body+'{end}';
export function sourceURL(settings,auto,name){const a=settings?.admin||{},host=settings?.stream?.obs_read_host||'';const loop=/^127\./.test(host)||['localhost','::1'].includes(host.toLowerCase());const selected=a.lan?(host||auto||'127.0.0.1'):'127.0.0.1';return {url:'http://'+(selected.includes(':')?'['+selected+']':selected)+':'+(a.port||8787)+'/source/'+encodeURIComponent(name),hint:!a.lan&&!!host&&!loop};}
