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
  'settings_camera 1（cam1 name） — settings_removed',
  'settings_camera 3（cam3 name） — settings_added',
  'settings_camera 2（cam2 name） — setting_name'
 ]);
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
test('revert confirmation includes the restored camera name from the ID diff',()=>{
 const lines=state.confirmationLines('revert',{},{
  apply:'restart',fields:[{path:'cameras[cam4]',kind:'added',camera_name:'Restored'}]
 },false,k=>k,{cameras:[]});
 assert.equal(lines[1],'settings_camera 4（Restored） — settings_added');
});
test('add button reasons distinguish camera limit from exhausted draft IDs',()=>{
 const all=cams(...Array.from({length:8},(_,i)=>'cam'+(i+1)));
 assert.equal(state.cameraAddReason(all,all,[]),'settings_camera_limit');
 assert.equal(state.cameraAddReason(all,cams('cam1'),[]),'settings_camera_no_id');
 assert.equal(state.cameraAddReason([],[],[]),'');
});
