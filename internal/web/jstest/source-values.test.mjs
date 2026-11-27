import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as state from '../assets/app/sources-state.js';
const translations={
 'src_values_current_camera':'今のシーンのカメラ','src_values_camera':'カメラ: {id}（{name}）',
 'src_values_standby':'待機','src_values_obs':'OBS','src_values_stream':'配信','src_values_other':'その他',
 'src_value_camera.kbps':'受信のビットレート（kbps）','src_value_camera.state':'状態（OK / LOW / DOWN）',
 'src_value_obs.connected':'OBS connection','src_value_now':'今の時刻',
};
const t=key=>translations[key]||key;
const groups=[
 {kind:'current_camera',values:[{name:'camera.kbps',text:'4200'},{name:'camera.state',text:null}]},
 {kind:'camera',camera:{id:'rear-cam',name:'後方'},values:[{name:'camera.rear-cam.kbps',text:'600'}]},
 {kind:'obs',values:[{name:'obs.connected',text:'false'}]},
 {kind:'other',values:[{name:'now',text:''}]},
];
test('value groups localize camera meanings and headings, preserve order and distinguish null from empty',()=>{
 const rows=state.sourceValueGroups(groups,'',t);
 assert.deepEqual(rows.map(g=>g.key),['current_camera','camera:rear-cam','obs','other']);
 assert.equal(rows[1].title,'カメラ: rear-cam（後方）');
 assert.equal(rows[0].title,'今のシーンのカメラ');
 assert.equal(rows[1].values[0].meaning,'受信のビットレート（kbps）');
 assert.equal(rows[0].values[1].text,'—');
 assert.equal(rows[2].values[0].text,'false');
 assert.equal(rows[3].values[0].text,'');
 assert.equal(rows[0].values[0].label,'camera.kbps: 受信のビットレート（kbps）');
 for(const kind of ['standby','obs','stream','other'])assert.equal(state.sourceValueGroups([{kind,values:[{name:'now',text:'12:34'}]}],'',t)[0].title,t('src_values_'+kind));
 assert.equal(groups[0].values[1].text,null);
});
test('filter matches name or localized meaning ignoring case, hides empty groups and ignores current text',()=>{
 for(const [query,want] of [['CAMERA.KBPS',['camera.kbps']],['ビットレート',['camera.kbps','camera.rear-cam.kbps']],['CONNECTION',['obs.connected']],['4200',[]],['nothing',[]]]) {
  assert.deepEqual(state.sourceValueGroups(groups,query,t).flatMap(g=>g.values.map(v=>v.name)),want);
 }
 assert.deepEqual(state.sourceValueGroups([],'',t),[]);
});
test('camera heading substitution preserves placeholder-like names and dollar characters',()=>{
 const g={kind:'camera',camera:{id:'front',name:'{id} $&'},values:[{name:'camera.front.kbps',text:'0'}]};
 assert.equal(state.sourceValueGroups([g],'',t)[0].title,'カメラ: front（{id} $&）');
});
test('value rows insert the exact template token and retain the existing cursor rules',()=>{
 for(const name of ['camera.kbps','camera.rear-cam.srt.loss','now']) {
  const text=state.valueText(name);assert.equal(text,'{'+name+'}');
  assert.deepEqual(state.editorInsertion('abc',{from:0,to:0},false,text),{from:3,to:3,insert:text});
  assert.deepEqual(state.editorInsertion('abc',{from:1,to:2},true,text),{from:1,to:2,insert:text});
 }
});
test('closing editor clears values state and errors separately from list notices',()=>{
 const s={valueGroups:groups,valuesOpen:true,valuesFilter:'abc',valuesError:'unavailable',notice:'kept'};
 state.clearEditor(s);
 assert.deepEqual(s.valueGroups,[]);assert.equal(s.valuesOpen,false);assert.equal(s.valuesFilter,'');assert.equal(s.valuesError,'');assert.equal(s.notice,'kept');
});
