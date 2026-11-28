import {test} from 'node:test';
import assert from 'node:assert/strict';
import {rebaseDraft,projectPage,pageDirty} from '../assets/app/settings-pages.js';
test('pages project leaves rather than top-level containers',()=>{
 const locations={'stream.app':{page:'cameras'},'stream.read_user':{page:'settings'}};
 assert.deepEqual(projectPage({stream:{app:'live',read_user:'reader'}},'cameras',locations),{stream:{app:'live'}});
});
test('rebase keeps independent edits and discards conflicts by camera id',()=>{
 const base={cameras:[{id:'cam1',name:'a'},{id:'cam2',name:'b'}],log:{keep_days:1}};
 const draft=structuredClone(base);draft.cameras[0].name='mine';draft.log.keep_days=2;
 const next={cameras:[{id:'cam2',name:'b'},{id:'cam1',name:'external'}],log:{keep_days:1}};
 const result=rebaseDraft(base,draft,next);
 assert.equal(result.values.log.keep_days,2);assert.equal(result.values.cameras.find(c=>c.id==='cam1').name,'external');assert.deepEqual(result.conflicts,['cameras[cam1].name']);
 assert.equal(pageDirty(result.values,next),true);assert.equal(pageDirty(next,next),false);
});
test('camera membership rebase keeps additions and refuses concurrently edited deletion',()=>{
 const base={cameras:[{id:'cam1',name:'a'}]};
 assert.deepEqual(rebaseDraft(base,{cameras:[...base.cameras,{id:'cam2',name:'new'}]},base).values.cameras.map(c=>c.id),['cam1','cam2']);
 const r=rebaseDraft(base,{cameras:[]},{cameras:[{id:'cam1',name:'updated'}]});
 assert.deepEqual(r.conflicts,['cameras[cam1]']);assert.equal(r.values.cameras[0].name,'updated');
});
test('drafts are independent and both dirty flags survive navigation',()=>{
 const baseline={stream:{app:'live',read_user:'reader'}};
 const locations={'stream.app':{page:'cameras',group:'common'},'stream.read_user':{page:'settings',group:'stream'}};
 const camera=projectPage(baseline,'cameras',locations),settings=projectPage(baseline,'settings',locations);
 const draft=structuredClone(camera);draft.stream.app='new';
 assert.equal(pageDirty(draft,camera),true);assert.equal(pageDirty(settings,projectPage(baseline,'settings',locations)),false);
});
