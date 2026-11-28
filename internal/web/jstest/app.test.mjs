import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createStore} from '../assets/app/store.js';
import {applyEvent,connectEvents} from '../assets/app/events.js';
import {apiError,createAPI} from '../assets/app/api.js';
import {selectLanguage} from '../assets/app/i18n.js';
test('updates are immutable and notifications coalesce',async()=>{const s=createStore({n:0});let calls=0;const unsubscribe=s.subscribe(v=>{calls++;assert.equal(v.n,2);});s.update(x=>x.n++);s.update(x=>x.n++);assert.equal(s.get().n,2);assert.throws(()=>s.get().n=3);await Promise.resolve();assert.equal(calls,1);unsubscribe();s.update(x=>x.n++);await Promise.resolve();assert.equal(calls,1);});
test('SSE changes status without erasing notices',()=>{const s=createStore({notice:'saved',senderNotice:'copied',status:null,readings:{},flashes:{},cameraStates:{}});const status={obs:{connected:true},cameras:[]};applyEvent(s,'status',status);assert.deepEqual(s.get().status,status);assert.equal(s.get().notice,'saved');assert.equal(s.get().senderNotice,'copied');});
test('API preserves 422 field paths and redirects unauthorized requests',async()=>{assert.deepEqual(apiError({fields:{'cameras[cam1].name':'invalid'}},422,'failed').fields,{'cameras[cam1].name':'invalid'});let url,options,redirect;const request=createAPI({lang:'ja',t:()=>'',login:false},async(u,o)=>{url=u;options=o;return {status:401,ok:false,json:async()=>({error:'unauthorized'})};},u=>redirect=u);await assert.rejects(request('settings',{values:{}},'PUT'));assert.equal(options.headers['X-IRL-Live-Keeper'],'1');assert.equal(options.method,'PUT');assert.equal(url,'/api/settings?lang=ja');assert.equal(redirect,'/?login');});
test('EventSource owns reconnect and error does not erase notice',()=>{let source;class Source{constructor(url){this.url=url;source=this;}addEventListener(){}close(){this.closed=true;}}const s=createStore({notice:'saved'});const close=connectEvents(s,'en',Source);assert.equal(source.url,'/api/events?topics=status&lang=en');source.onerror();assert.equal(s.get().notice,'saved');assert.equal(s.get().disconnected,true);close();assert.ok(source.closed);});
test('language precedence',()=>{assert.equal(selectLanguage('ja','en',['en']),'ja');assert.equal(selectLanguage(null,'en',['ja']),'en');assert.equal(selectLanguage(null,'auto',['ja-JP']),'ja');});
import {dirty,fieldGroup,confirmationLines,confirmationTitle} from '../assets/app/settings-state.js';
test('settings dirty compares nested values and returns to baseline',()=>{const values={cameras:[{name:'a'}],admin:{lan:false}},s={data:{},values,baseline:JSON.stringify(values)};assert.equal(dirty(s),false);values.cameras[0].name='b';assert.equal(dirty(s),true);values.cameras[0].name='a';assert.equal(dirty(s),false);values.admin.lan=true;assert.equal(dirty(s),true);});
test('422 field paths select their settings group',()=>{const e=apiError({fields:{'cameras[cam3].name':'bad','stream.publish_pass':'bad','mediamtx.binary':'bad','language':'bad'}},422,'failed');assert.deepEqual(Object.keys(e.fields).map(fieldGroup),['camera','common','admin','other']);});
test('confirmation identifies password target and warns only for restart during stream',()=>{for(const target of ['admin','publish','read']){const lines=confirmationLines('regenerate',{target},{apply:target==='admin'?'immediate':'restart'},true,k=>k);assert.equal(confirmationTitle('regenerate',{target},k=>k),'confirm_regenerate_'+target);assert.ok(!lines.includes('settings_regenerate_'+target));assert.equal(lines.includes('settings_stream_warning'),target!=='admin');}assert.deepEqual(confirmationLines('save',{}, {apply:'restart',fields:[{path:'cameras[cam3].name'}],admin_lan_will_close:true},false,k=>k),['settings_restart_mode','settings_camera 3 — setting_name','settings_lan_close']);});
test('SSE leaves settings operation notice and edits intact',()=>{const s=createStore({notice:'settings_applied',values:{admin:{port:99}},baseline:'saved',errors:{'admin.port':'invalid'},readings:{},flashes:{},cameraStates:{}});applyEvent(s,'status',{obs:{connected:true},cameras:[]});assert.equal(s.get().notice,'settings_applied');assert.equal(s.get().values.admin.port,99);assert.equal(s.get().errors['admin.port'],'invalid');});
test('settings request failures preserve status and message for 403 409 500',async()=>{for(const status of [403,409,500]){const request=createAPI({lang:'en',t:()=> 'failed',login:false},async()=>({status,ok:false,json:async()=>({code:'api.failure',error:'failure '+status})}));await assert.rejects(request('settings',{values:{}},'PUT'),e=>e.status===status&&e.message==='failure '+status);}});
import {readFileSync,existsSync,readdirSync} from 'node:fs';
test('all application imports resolve to bundled files',()=>{
 const dir=new URL('../assets/app/',import.meta.url);
 const page=readFileSync(new URL('../assets/page.html',import.meta.url),'utf8');
 const imports=JSON.parse(page.match(/<script type="importmap">([\s\S]*?)<\/script>/)[1]).imports;
 for(const name of readdirSync(dir).filter(n=>n.endsWith('.js'))){
  const source=readFileSync(new URL(name,dir),'utf8');
  for(const match of source.matchAll(/(?:from\s*|import\s*\()['"]([^'"]+)['"]/g)){
   const path=match[1].startsWith('.')?match[1]:imports[match[1]]?.replace('/assets/','../');
   assert.ok(path&&existsSync(new URL(path,dir)),name+': '+match[1]);
  }
 }
});
