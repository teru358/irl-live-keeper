import {test} from 'node:test';
import assert from 'node:assert/strict';
import {layout,remember,dirty,assetText,valueText,conditionText,previewRequest,sourceURL,sourceFileDeletionPaused} from '../assets/app/sources-state.js';
test('source file deletion is paused',()=>{assert.equal(sourceFileDeletionPaused(),true);});
test('layout survives storage failure and rejects invalid values',()=>{assert.deepEqual(layout({getItem(){throw Error();}}),{mode:'split',axis:'horizontal'});assert.deepEqual(layout({getItem:()=>'{"mode":"bad","axis":"vertical"}'}),{mode:'split',axis:'vertical'});remember({setItem(){throw Error();}},{});let saved;remember({setItem(k,v){saved=v;}},{mode:'previewonly',axis:'vertical'});assert.equal(layout({getItem:()=>saved}).mode,'previewonly');});
test('insertions and unsaved changes',()=>{assert.equal(assetText('a.png'),'url("/source/files/a.png")');assert.equal(valueText('cam1.kbps'),'{cam1.kbps}');assert.equal(conditionText('cam1.online','x'),'{if cam1.online}x{end}');assert.equal(dirty({css:'a'},{css:'a'}),false);assert.equal(dirty({css:'b'},{css:'a'}),true);});
test('preview payload distinguishes draft and names',()=>{assert.deepEqual(previewRequest(null,['a'],{}),{names:['a'],simulate:{}});assert.deepEqual(previewRequest({template:'x'},[],{line:'LOW'}),{source:{template:'x'},simulate:{line:'LOW'}});});

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
test('insertions append until cursor was placed, with a complete CSS rule for assets',()=>{
 assert.deepEqual(state.editorInsertion('abc',{from:0,to:0},false,'X'),{from:3,to:3,insert:'X'});
 assert.deepEqual(state.editorInsertion('abc',{from:1,to:2},true,'X'),{from:1,to:2,insert:'X'});
 assert.deepEqual(state.editorInsertion('.irl-source {}',{from:0,to:0},false,'a b.png',true),{from:14,to:14,insert:'\n.irl-source { background-image: url("/source/files/a%20b.png"); }'});
 assert.equal(state.editorInsertion('',{from:0,to:0},false,'a.png',true).insert,'.irl-source { background-image: url("/source/files/a.png"); }');
 assert.equal(state.editorInsertion('a\n',{from:0,to:0},false,'a.png',true).insert,'.irl-source { background-image: url("/source/files/a.png"); }');
 assert.equal(state.editorInsertion('abc',{from:1,to:1},true,'a.png',true).insert,assetText('a.png'));
});
test('empty decoration fields use zero numbers and empty colors without changing wire shape',()=>{
 assert.equal(state.decorationInput(0),'');assert.equal(state.decorationInput(''),'');
 assert.equal(state.decorationInput(-12),'-12');assert.equal(state.decorationInput('#000000'),'#000000');
 assert.equal(state.decorationNumber(''),0);assert.equal(state.decorationNumber('0'),0);assert.equal(state.decorationNumber('-3.5'),-3.5);
});
test('appearance changes exclude template, title, name and OBS dimensions',()=>{
 const base={name:'a',title:'A',template:'x',style:{color:''},css:'',obs_width:800,obs_height:200};
 for(const [key,value] of Object.entries({name:'b',title:'B',template:'y',obs_width:900,obs_height:300}))assert.equal(state.appearanceChanged({...base,[key]:value},base),false);
 assert.equal(state.appearanceChanged({...base,css:'new'},base),true);
 assert.equal(state.appearanceChanged({...base,style:{color:'#000000'}},base),true);
});
test('preview errors belong to editor and clear on success',()=>{
 const s={draft:{},notice:'kept',editorNotice:'saved',preview:null,previewError:''};
 state.applyPreview(s,{template_error:{error:'bad',line:1,column:2,code:'bad'}});
 assert.equal(s.previewError,'bad');assert.equal(s.notice,'kept');assert.equal(s.preview,null);
 state.applyPreview(s,{text:'ok',css:''});assert.equal(s.previewError,'');assert.equal(s.preview.text,'ok');assert.equal(s.editorNotice,'saved');
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
test('preview changes keep stylesheet for text ticks and only reload after source save',()=>{
 const first={name:'a',text:'one',version:0};
 assert.deepEqual(state.previewChanges(null,first),{text:true,style:true});
 assert.deepEqual(state.previewChanges(first,{...first,text:'two'}),{text:true,style:false});
 assert.deepEqual(state.previewChanges(first,{...first}),{text:false,style:false});
 assert.deepEqual(state.previewChanges(first,{...first,version:1}),{text:false,style:true});
 const editor={name:'a',text:'one',css:'a{}'};
 assert.equal(state.previewChanges(editor,{...editor,text:'two'}).style,false);
 assert.equal(state.previewChanges(editor,{...editor,css:'b{}'}).style,true);
});
test('delete prompts identify source titles and asset names, with fallback for missing sources',()=>{
 const list=[{name:'a',title:'回線の表示'},{name:'b',title:''}];
 const t=key=>({src_confirmdelete:'「{name}」を削除しますか？',src_used:'使用中のソース: {sources}'})[key];
 assert.equal(state.deletePrompt(list[0],list,t),'「回線の表示」を削除しますか？');
 assert.equal(state.deletePrompt({name:'image.png',used_by:['a','b','gone']},list,t),'「image.png」を削除しますか？\n使用中のソース: 回線の表示, b, gone');
});
test('closing editor clears errors, preview and feedback without touching list notice',()=>{
 const s={draft:{},base:{},preview:{text:'x'},previewError:'bad',editorError:'error',editorNotice:'saved',notice:'kept',styleInputs:{padding:'0'}};
 state.clearEditor(s);
 for(const key of ['draft','base','preview'])assert.equal(s[key],null);
 for(const key of ['previewError','editorError','editorNotice'])assert.equal(s[key],'');
 assert.deepEqual(s.styleInputs,{});assert.equal(s.notice,'kept');
});
test('incomplete template retains last stylesheet until next successful preview',()=>{
 const s={draft:{},preview:{text:'last',css:'color{}'},previewError:''};
 state.applyPreview(s,{template_error:{error:'bad'}});
 assert.deepEqual(s.preview,{text:'last',css:'color{}'});
 state.applySourceEvent(s,{sources:{},test_obs:{enabled:false,reason:'stream_started'}},'stopped');
 assert.equal(s.preview.css,'color{}');assert.equal(s.preview.text,'');
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
 const src={name:'bad',template:'{link.kbps}',style:{}};
 state.openEditor(s,src);
 assert.equal(s.previewError,error.error);assert.equal(s.preview,null);
 assert.deepEqual(s.diagnostics,state.templateDiagnostics(error));
 assert.deepEqual(s.draft,src);assert.equal(dirty(s.draft,s.base),false);
 s.draft.template='fixed';assert.equal(s.base.template,'{link.kbps}');
 assert.equal(s.notice,'kept');
 state.applyPreview(s,{text:'fixed',css:''});
 assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
 state.applyPreview(s,{template_error:error});
 assert.equal(s.previewError,'1 行 2 文字目: 知らない値の名前です');
 assert.deepEqual(s.diagnostics,state.templateDiagnostics(error));
 state.clearEditor(s);assert.deepEqual(s.diagnostics,[]);
 state.openEditor(s,{name:'good',template:'hello',style:{}});
 assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
});
test('collection loads preserve source JSON, refresh broken markers and clear stale preview text',()=>{
 const source={name:'bad',template:'{link.kbps}'};
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
  state.openEditor(s,{name,template:'hello'});
  assert.equal(s.previewError,'');assert.deepEqual(s.diagnostics,[]);
 }
});
