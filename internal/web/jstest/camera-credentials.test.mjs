import test from 'node:test';
import assert from 'node:assert/strict';
import {projectPage} from '../assets/app/settings-pages.js';
test('camera password never enters page draft or save',()=>{
 const values={cameras:[{id:'cam1',name:'Main',publish_pass:'private'}]};
 const result=projectPage(values,'cameras',{'cameras[].id':{page:'cameras'},'cameras[].publish_pass':{page:'cameras'}});
 assert.equal(Object.hasOwn(result.cameras[0],'publish_pass'),false);
 assert.equal(values.cameras[0].publish_pass,'private');
});

import {confirmationLines} from '../assets/app/settings-state.js';
test('publish confirmation identifies target and warns about every camera',()=>{
 const lines=confirmationLines('regenerate',{target:'publish',camera:'cam2'},{apply:'restart'},false,k=>k);
 assert.ok(lines.includes('settings_camera 2'));
 assert.ok(lines.includes('publish_reset_warning'));
});
