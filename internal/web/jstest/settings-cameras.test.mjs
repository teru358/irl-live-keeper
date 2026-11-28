import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as state from '../assets/app/settings-state.js';
const cams=(...ids)=>ids.map(id=>({id,name:id+' name'}));
test('allocate smallest free camera ID across saved, draft and removed IDs',()=>{
 const allocate=state.nextCameraID;
 assert.equal(typeof allocate,'function');
 assert.equal(allocate([],[],[]),'cam1');
 assert.equal(allocate(cams('cam1','cam3'),cams('cam1','cam3'),[]),'cam2');
 assert.equal(allocate(cams('cam1','cam2'),cams('cam1'),[]),'cam3');
 assert.equal(allocate(cams('cam1'),cams('cam1'),['cam2']),'cam3');
 assert.equal(allocate(cams('cam1','cam2'),cams('cam1','cam3'),['cam4']),'cam5');
 // A new saved baseline permits reuse after a completed save.
 assert.equal(allocate(cams('cam2'),cams('cam2'),[]),'cam1');
 const all=cams(...Array.from({length:8},(_,i)=>'cam'+(i+1)));
 assert.equal(allocate(all,all,[]),null);
 assert.equal(allocate(all,cams('cam1'),[]),null);
 assert.equal(allocate([],cams('cam1','cam2','cam3','cam4','cam5','cam6','cam7'),['cam8']),null);
});
test('confirmation camera headings use IDs and names for edits, adds and deletes',()=>{
 const payload={values:{cameras:cams('cam3','cam2')}};
 const saved={cameras:cams('cam1','cam2')};
 const lines=state.confirmationLines('save',payload,{apply:'restart',fields:[
  {path:'cameras[cam1]',kind:'removed'},{path:'cameras[cam3]',kind:'added'},{path:'cameras[cam2].name'}
 ]},false,k=>k,saved);
 assert.deepEqual(lines,[
  'settings_restart_mode',
  'settings_camera 1 (cam1 name) — settings_removed',
  'settings_camera 3 (cam3 name) — settings_added',
  'settings_camera 2 (cam2 name) — setting_name'
 ]);
});
const usageText={settings_camera_used:'このカメラの値を使っているソース: {sources}',settings_camera_used_note:'直しが要る状態になり、OBS のページは空になります。',settings_camera_sources_unreadable:'ソースの一覧が読めないため、このカメラを使っているソースを確かめられません'};
const tu=k=>usageText[k]||k;
test('camera delete confirmation lists source titles with the repair and empty-page note',()=>{
 const saved={cameras:cams('cam1','cam2')};
 const usage=state.cameraUsage({sources:[
  {name:'a',title:'回線の表示',cameras:['cam1']},
  {name:'b',title:'',cameras:['cam1','cam2']},
  {name:'c',title:'無関係',cameras:[]}
 ]});
 const preview={apply:'restart',fields:[{path:'cameras[cam1]',kind:'removed'}]};
 assert.deepEqual(state.removedCameraIDs(preview),['cam1']);
 const lines=state.confirmationLines('save',{values:{cameras:cams('cam2')}},preview,false,tu,saved,usage);
 assert.deepEqual(lines.slice(1),[
  'settings_camera 1 (cam1 name) — settings_removed',
  'settings_camera 1 (cam1 name) — このカメラの値を使っているソース: 回線の表示, b',
  '直しが要る状態になり、OBS のページは空になります。'
 ]);
 assert.ok(!lines.join('\n').includes('無関係'));
 // Only a removed camera is checked: an edit or an addition adds nothing.
 const edit=state.confirmationLines('save',{values:{cameras:cams('cam1','cam2')}},{apply:'restart',fields:[{path:'cameras[cam1].name'},{path:'cameras[cam3]',kind:'added'}]},false,tu,saved,usage);
 assert.ok(!edit.join('\n').includes('このカメラの値'));
 // A removed camera that no source uses has no note.
 const none=state.confirmationLines('save',{values:{cameras:cams('cam1')}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'}]},false,tu,saved,state.cameraUsage({sources:[{name:'a',title:'a',cameras:['cam1']}]}));
 assert.deepEqual(none.slice(1),['settings_camera 2 (cam2 name) — settings_removed']);
});
test('camera delete confirmation says the source list cannot be read, and the deletion still goes on',()=>{
 const saved={cameras:cams('cam1')};
 const preview={apply:'restart',fields:[{path:'cameras[cam1]',kind:'removed'}]};
 for(const data of [null,{code:'sources.unreadable',sources:[],error:'x'},{}]){
  const usage=state.cameraUsage(data);
  assert.equal(usage.error,true);
  const lines=state.confirmationLines('save',{values:{cameras:[]}},preview,false,tu,saved,usage);
  assert.deepEqual(lines.slice(1),['settings_camera 1 (cam1 name) — settings_removed','ソースの一覧が読めないため、このカメラを使っているソースを確かめられません']);
 }
 // Without a usage lookup the lines are as before.
 assert.deepEqual(state.confirmationLines('save',{values:{cameras:[]}},preview,false,tu,saved).slice(1),['settings_camera 1 (cam1 name) — settings_removed']);
});
test('422 focuses the camera field by ID and opens its details',()=>{
 const path='cameras[cam3].switch.low_sec',events=[];
 const details={open:false},input={closest:()=>details,focus:()=>events.push('focus'),scrollIntoView:()=>events.push('scroll')};
 const doc={getElementById:id=>{assert.equal(id,'setting-'+path);return input;}};
 assert.equal(state.fieldGroup(path),'camera');
 assert.equal(typeof state.focusSettingError,'function');
 state.focusSettingError({[path]:'bad'},doc);
 assert.equal(details.open,true);assert.deepEqual(events,['focus','scroll']);
});
test('confirmation includes the camera name from the ID diff',()=>{
 const lines=state.confirmationLines('save',{},{
  apply:'restart',fields:[{path:'cameras[cam4]',kind:'added',camera_name:'Restored'}]
 },false,k=>k,{cameras:[]});
 assert.equal(lines[1],'settings_camera 4 (Restored) — settings_added');
});
test('add button reasons distinguish camera limit from exhausted draft IDs',()=>{
 const all=cams(...Array.from({length:8},(_,i)=>'cam'+(i+1)));
 assert.equal(state.cameraAddReason(all,all,[]),'settings_camera_limit');
 assert.equal(state.cameraAddReason(all,cams('cam1'),[]),'settings_camera_no_id');
 assert.equal(state.cameraAddReason([],[],[]),'');
});

test('camera headings support empty and named cameras in both languages',()=>{
 for(const [word,empty,named] of [['カメラ','カメラ 8','カメラ 8（後方）'],['Camera','Camera 8','Camera 8 (後方)']]) {
 const t=()=>word;
 assert.equal(state.cameraHeading({id:'cam8',name:''},t),empty);
 assert.equal(state.cameraHeading({id:'cam8',name:'後方'},t),named);
 }
});
