import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as state from '../assets/app/sources-state.js';
const t=key=>({'settings_camera':'カメラ','src_value_cam<n>.kbps':'受信 kbps','src_values_obs':'OBS'}[key]||key);
const groups=[{kind:'camera',camera:{id:'cam2',name:''},values:[{name:'cam2.kbps',text:null}]},{kind:'obs',values:[{name:'obs.connected',text:'false'}]}];
test('camera groups use numbered heading and shared meaning keys',()=>{
 const rows=state.sourceValueGroups(groups,'',t);
 assert.deepEqual(rows.map(g=>g.key),['camera:cam2','obs']);
 assert.equal(rows[0].title,'カメラ 2');assert.equal(rows[0].values[0].meaning,'受信 kbps');assert.equal(rows[0].values[0].text,'—');
 assert.equal(state.sourceValueGroups(groups,'CAM2',t).length,1);
 assert.equal(state.sourceValueGroups(groups,'受信',t).length,1);
});
test('value rows insert the exact template token and retain the existing cursor rules',()=>{
 for(const name of ['cam1.kbps','cam2.srt.loss','now']) {
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
