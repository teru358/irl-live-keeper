import {test} from 'node:test';
import assert from 'node:assert/strict';
import {layout,remember,dirty,valueText,conditionText,previewRequest,sourceURL} from '../assets/app/sources-state.js';
test('layout survives storage failure and rejects invalid values',()=>{assert.deepEqual(layout({getItem(){throw Error();}}),{mode:'split',axis:'horizontal'});assert.deepEqual(layout({getItem:()=>'{"mode":"bad","axis":"vertical"}'}),{mode:'split',axis:'vertical'});remember({setItem(){throw Error();}},{});let saved;remember({setItem(k,v){saved=v;}},{mode:'previewonly',axis:'vertical'});assert.equal(layout({getItem:()=>saved}).mode,'previewonly');});
test('insertions and unsaved changes',()=>{assert.equal(valueText('cam1.kbps'),'{cam1.kbps}');assert.equal(conditionText('cam1.online','x'),'{if cam1.online}x{end}');assert.equal(dirty({body:'a'},{body:'a'}),false);assert.equal(dirty({body:'b'},{body:'a'}),true);});
test('preview payload distinguishes draft and names',()=>{assert.deepEqual(previewRequest(null,['a'],{}),{names:['a'],simulate:{}});assert.deepEqual(previewRequest({body:'x'},[],{line:'LOW'}),{source:{body:'x'},simulate:{line:'LOW'}});});

import * as state from '../assets/app/sources-state.js';
test('source URL host table always includes local and optional non-loopback remote',()=>{
 for(const lan of [false,true])for(const host of ['', 'localhost','LOCALHOST','127.9.2.1','::1','[::1]','relay.local','2001:db8::1','[2001:db8::1]'])for(const auto of ['', '192.168.1.3']){
  const remoteHost=['relay.local','2001:db8::1','[2001:db8::1]'].includes(host)?host:auto;
  const r=sourceURL({admin:{lan,port:9000},stream:{obs_read_host:host}},auto,'a b');
  assert.equal(r.url,'http://127.0.0.1:9000/source/a%20b');
  const expected=lan&&remoteHost?'http://'+(remoteHost.includes(':')?'['+remoteHost.replace(/^\[|\]$/g,'')+']':remoteHost)+':9000/source/a%20b':null;
  assert.equal(r.remoteURL,expected,JSON.stringify({lan,host,auto}));
  assert.equal(r.hint,!lan&&['relay.local','2001:db8::1','[2001:db8::1]'].includes(host));
 }
});
test('insertions append until the cursor was placed, and replace the selection after',()=>{
 assert.deepEqual(state.editorInsertion('abc',{from:0,to:0},false,'X'),{from:3,to:3,insert:'X'});
 assert.deepEqual(state.editorInsertion('abc',{from:1,to:2},true,'X'),{from:1,to:2,insert:'X'});
 assert.deepEqual(state.editorInsertion('abc',{from:1,to:3},true,'日本'),{from:1,to:3,insert:'日本'});
 const apply=(value,r)=>value.slice(0,r.from)+r.insert+value.slice(r.to);
 assert.equal(apply('a<b>c',state.editorInsertion('a<b>c',{from:2,to:2},true,state.valueText('now'))),'a<{now}b>c');
});
test('a new source starts with an empty body and the wire shape is exactly name, title, body and OBS size',()=>{
 const s={defaults:{name:'',title:'',body:'',obs_width:800,obs_height:200},broken:{},texts:{}};
 state.openEditor(s,null);
 assert.equal(s.fresh,true);assert.equal(s.draft.body,'');
 assert.deepEqual(Object.keys(s.draft).sort(),['body','name','obs_height','obs_width','title']);
 assert.equal(dirty(s.draft,s.base),false);
 s.draft.name='demo';s.draft.body='<p>{cam1.kbps}</p>';
 assert.equal(dirty(s.draft,s.base),true);
 // Save posts the draft as is; the preview request carries the same body.
 assert.deepEqual(JSON.parse(JSON.stringify(s.draft)),{name:'demo',title:'',body:'<p>{cam1.kbps}</p>',obs_width:800,obs_height:200});
 assert.deepEqual(previewRequest(s.draft,[],{}).source.body,'<p>{cam1.kbps}</p>');
 assert.equal('template' in s.draft||'css' in s.draft||'style' in s.draft,false);
});
test('preview errors belong to editor and clear on success',()=>{
 const s={draft:{},notice:'kept',editorNotice:'saved',preview:null,previewError:''};
 state.applyPreview(s,{template_error:{error:'bad',line:1,column:2,code:'bad'}});
 assert.equal(s.previewError,'bad');assert.equal(s.notice,'kept');assert.equal(s.preview,null);
 state.applyPreview(s,{text:'ok'});assert.equal(s.previewError,'');assert.equal(s.preview.text,'ok');assert.equal(s.editorNotice,'saved');
});
test('stream start resets all axes and free without overwriting operation notice; timeout and disabled do not',()=>{
 for(const reason of ['stream_started','timeout','disabled']){
  const reset=reason==='stream_started';
  const s={test:true,line:'OK',standby:'auto',stream:'offline',free:true,custom:'{"cam1.kbps":1}',texts:{a:'fake'},notice:'kept',testNotice:'',testReason:''};
  const got=state.applySourceEvent(s,{sources:{a:'real'},test_obs:{enabled:false,reason}},'stopped');
  assert.equal(got,reset);
  for(const [axis,value] of [['line','OK'],['standby','auto'],['stream','offline']])assert.equal(s[axis],reset?'actual':value);
  assert.equal(s.free,!reset);assert.equal(s.custom,'{"cam1.kbps":1}');
  assert.equal(state.testActive(s),!reset);
  assert.equal(s.texts.a,reset?'real':'fake');assert.equal(s.notice,'kept');
  assert.equal(s.testNotice,reset?'stopped':'');
  s.line='LOW';assert.equal(state.applySourceEvent(s,{sources:{a:'real'},test_obs:{enabled:false,reason}},'stopped'),false);assert.equal(s.line,'LOW');
 }
});
test('three test choices: each axis has an actual, labels and the simulate object',()=>{
 assert.deepEqual(state.testAxes,{line:['actual','OK','LOW','DOWN'],standby:['actual','none','auto'],stream:['actual','streaming','offline']});
 assert.deepEqual(state.testInitial(),{line:'actual',standby:'actual',stream:'actual',free:false,custom:'{}'});
 assert.equal(state.testChoiceKey('line','actual'),'src_actual');assert.equal(state.testChoiceKey('standby','auto'),'src_standby_auto');assert.equal(state.testChoiceKey('stream','offline'),'src_stream_offline');
 const s=state.testInitial();
 assert.deepEqual(state.simulateOf(s),{});assert.equal(state.testActive(s),false);
 s.line='DOWN';assert.deepEqual(state.simulateOf(s),{line:'DOWN'});assert.equal(state.testActive(s),true);
 s.standby='none';s.stream='streaming';assert.deepEqual(state.simulateOf(s),{line:'DOWN',standby:'none',stream:'streaming'});
 assert.equal('mode' in state.simulateOf(s),false);
 // 自由は別の切り替え。オフなら JSON を送らない。オンなら values に入れる。
 s.custom='{"cam1.kbps":300,"cam2.state":"DOWN"}';assert.equal('values' in state.simulateOf(s),false);
 s.free=true;assert.deepEqual(state.simulateOf(s).values,{'cam1.kbps':300,'cam2.state':'DOWN'});
 s.line=s.standby=s.stream='actual';assert.equal(state.testActive(s),true);assert.deepEqual(state.simulateOf(s),{values:{'cam1.kbps':300,'cam2.state':'DOWN'}});
 // 自由が空の値や読めない JSON なら、軸がすべて実際のときは「テスト中」ではない（OBS にも反映は使えない）。
 for(const custom of ['{}','','{','[1]','null'])assert.equal(state.testActive({...s,custom}),false,custom);
 assert.throws(()=>state.simulateOf({...s,custom:'{'}));
 assert.deepEqual(state.freeValues('{"a":1}'),{a:1});assert.equal(state.freeValues('[1]'),null);assert.equal(state.freeValues('x'),null);
 // 未設定の軸は実際として扱う（古い状態の入れ物でも壊れない）。
 assert.equal(state.testActive({}),false);assert.deepEqual(state.simulateOf({}),{});
});
test('testReset sets every axis to actual and turns free off, keeping the typed JSON',()=>{
 const s={line:'LOW',standby:'auto',stream:'streaming',free:true,custom:'{"a":1}'};
 state.testReset(s);
 assert.deepEqual(s,{line:'actual',standby:'actual',stream:'actual',free:false,custom:'{"a":1}'});
 assert.equal(state.testSignature(s)===state.testSignature({...s,line:'OK'}),false);
});

test('delete confirmations identify source titles and asset names, with fallback for missing sources',()=>{
 const list=[{name:'a',title:'回線の表示'},{name:'b',title:''}];
 const t=key=>({src_confirmdelete:'「{name}」を削除しますか？',src_used:'使用中のソース: {sources}',src_file_used_note:'直しが要ります。OBS は空になります。'})[key];
 assert.deepEqual(state.deleteParts(list[0],list,t),{title:'「回線の表示」を削除しますか？',lines:[]});
 assert.deepEqual(state.deleteParts({name:'image.png',used_by:['a','b','gone']},list,t),{title:'「image.png」を削除しますか？',lines:['使用中のソース: 回線の表示, b, gone','直しが要ります。OBS は空になります。']});
 assert.deepEqual(state.deleteParts({name:'d/e.png',used_by:[]},list,t),{title:'「d/e.png」を削除しますか？',lines:[]});
});
test('the delete note says that the image stops showing and the editor warns, in both languages',async()=>{
 const {readFileSync}=await import('node:fs');
 for(const [lang,words] of [['ja',['画像が出なくなります','素材が見つかりません']],['en',['no longer show','warning']]]){
  const ui=JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
  for(const word of words)assert.ok(ui.src_file_used_note.includes(word),lang+' '+word);
  assert.ok(ui.src_used.includes('{sources}'));
 }
});
test('an edited source drops the camera ids of the list row, and warnings follow the preview',()=>{
 const row={name:'a',title:'t',body:'{cam1.kbps}',obs_width:800,obs_height:200,cameras:['cam1']};
 assert.deepEqual(state.sourceDraft(row),{name:'a',title:'t',body:'{cam1.kbps}',obs_width:800,obs_height:200});
 const s={broken:{},defaults:{obs_width:800,obs_height:200}};
 state.openEditor(s,row);
 assert.equal('cameras' in s.draft,false);
 assert.equal(JSON.stringify(s.draft),JSON.stringify(s.base));
 state.applyPreview(s,{text:'x',warnings:[{code:'template.asset_missing',message:'missing: a.png'}]});
 assert.deepEqual(s.warnings.map(w=>w.message),['missing: a.png']);
 state.applyPreview(s,{template_error:{error:'bad',line:1,column:1}});
 assert.deepEqual(s.warnings,[]);
 state.applyPreview(s,{text:'y',warnings:[{message:'w'}]});
 state.clearEditor(s);
 assert.deepEqual(s.warnings,[]);
});
test('closing editor clears errors, preview and feedback without touching list notice',()=>{
 const s={draft:{},base:{},preview:{text:'x'},previewError:'bad',editorError:'error',editorNotice:'saved',notice:'kept'};
 state.clearEditor(s);
 for(const key of ['draft','base','preview'])assert.equal(s[key],null);
 for(const key of ['previewError','editorError','editorNotice'])assert.equal(s[key],'');
 assert.equal(s.notice,'kept');
});
test('incomplete template retains last preview until stream reset',()=>{
 const s={draft:{},preview:{text:'last'},previewError:''};
 state.applyPreview(s,{template_error:{error:'bad'}});
 assert.deepEqual(s.preview,{text:'last'});
 state.applySourceEvent(s,{sources:{},test_obs:{enabled:false,reason:'stream_started'}},'stopped');
 assert.equal(s.preview.text,'');
});
test('mobile split hides axis choice and preserves desktop preference',()=>{
 const saved={mode:'split',axis:'horizontal'};
 assert.deepEqual(state.visibleLayout(saved,true),{axis:'vertical',showAxis:false});
 assert.deepEqual(saved,{mode:'split',axis:'horizontal'});
 assert.deepEqual(state.visibleLayout(saved,false),{axis:'horizontal',showAxis:true});
 assert.equal(state.visibleLayout({mode:'editonly',axis:'vertical'},false).showAxis,false);
});
test('saved broken sources expose the same positioned error as draft previews',()=>{
 const error={code:'template.unknown',error:'1 行 2 文字目: 知らない値の名前です',line:1,column:2};
 assert.deepEqual(state.templateDiagnostics(error),[{line:1,column:2,message:error.error}]);
 assert.deepEqual(state.templateDiagnostics(null),[]);
 const s={defaults:{obs_width:800},broken:{bad:error},notice:'kept',texts:{bad:'stale'}};
 const src={name:'bad',body:'{link.kbps}'};
 state.openEditor(s,src);
 assert.equal(s.previewError,error.error);assert.equal(s.preview,null);
 assert.deepEqual(s.diagnostics,state.templateDiagnostics(error));
 assert.deepEqual(s.draft,src);assert.equal(dirty(s.draft,s.base),false);
 s.draft.body='fixed';assert.equal(s.base.body,'{link.kbps}');
 assert.equal(s.notice,'kept');
 state.applyPreview(s,{text:'fixed'});
 assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
 state.applyPreview(s,{template_error:error});
 assert.equal(s.previewError,'1 行 2 文字目: 知らない値の名前です');
 assert.deepEqual(s.diagnostics,state.templateDiagnostics(error));
 state.clearEditor(s);assert.deepEqual(s.diagnostics,[]);
 state.openEditor(s,{name:'good',body:'hello'});
 assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
});
test('collection loads preserve source JSON, refresh broken markers and clear stale preview text',()=>{
 const source={name:'bad',body:'{link.kbps}'};
 const data={sources:[source],broken:{bad:{code:'template.unknown',error:'unknown',line:1,column:2}},names:['now'],default:{obs_width:800}};
 const s={texts:{bad:'stale'},notice:'kept'};
 state.applyCollection(s,data);
 assert.deepEqual(s.list,[source]);assert.deepEqual(s.broken,data.broken);
 assert.equal(s.texts.bad,'');assert.deepEqual(s.names,['now']);assert.equal(s.defaults.obs_width,800);
 assert.equal('broken' in s.list[0],false);assert.equal(s.notice,'kept');
 state.applyCollection(s,{...data,broken:{}});assert.deepEqual(s.broken,{});
});
test('valid source names that match Object properties do not need repair',()=>{
 for(const name of ['constructor','toString','__proto__']){
  const s={defaults:{},broken:{},texts:{}};
  state.openEditor(s,{name,body:'hello'});
  assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
 }
});
