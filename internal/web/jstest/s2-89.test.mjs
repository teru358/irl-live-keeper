// S2-89: the fixes after the Windows check S2-88.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {finishAccept,confirmReload} from '../assets/app/settings-flow.js';
import {sourceValueGroups,createFilesRefresher} from '../assets/app/sources-state.js';

const read=name=>readFileSync(new URL('../assets/app/'+name,import.meta.url),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
const t=key=>'T:'+key;

// ---- 1. the files are read again when the editor or the files section is opened ----
test('the file list is read again, and a read already running is shared',async()=>{
 let calls=0,release;
 const request=async path=>{calls++;assert.equal(path,'source-files');await new Promise(r=>{release=r;});return {files:[{name:'Sky.png',usable:true}],notices:[{message:'n'}]};};
 const state={files:[],fileNotices:[]};
 const refresh=createFilesRefresher({request,update:fn=>fn(state),fail:e=>{throw e;}});
 const a=refresh(),b=refresh();
 assert.equal(calls,1);
 release();await Promise.all([a,b]);
 assert.deepEqual(state.files.map(f=>f.name),['Sky.png']);assert.equal(state.fileNotices.length,1);
 const c=refresh();release();await c;
 assert.equal(calls,2);
});

test('a failed read of the file list is shown through fail',async()=>{
 const seen=[];
 const refresh=createFilesRefresher({request:async()=>{throw new Error('x');},update:()=>{},fail:e=>seen.push(e.message)});
 await refresh();
 assert.deepEqual(seen,['x']);
});

test('opening the editor and the files section read the list again; the plain list view does not',()=>{
 const source=read('sources.js');
 assert.match(source,/async function open\(src\)\{[^\n]*refreshFiles\(\)/);
 assert.match(source,/<details class="source-section"[^>]*@toggle=\$\{[^}]*refreshFiles/);
});

// ---- 2. the sentence after "restart" ----
test('"restart" says the restart was accepted, saving still says saved',async()=>{
 const run=async(action,result)=>{
  const s={baseline:'',notice:''};
  await finishAccept({action,result,values:{},preview:{},updateOBS:false,t,update:fn=>fn(s),dispatch:()=>{},load:async()=>{},closeDialog:()=>{},page:'settings'});
  return s.notice;
 };
 assert.equal(await run('restart',{}),'T:settings_restart_accepted');
 assert.equal(await run('save',{apply:'restart'}),'T:settings_saved');
 assert.equal(await run('regenerate',{apply:'restart'}),'T:settings_saved');
 assert.equal(await run('save',{apply:'immediate'}),'T:settings_applied');
 assert.equal(locale('ja').settings_restart_accepted,'再起動を受け付けました。');
 assert.equal(locale('en').settings_restart_accepted,'Restart accepted.');
});

// ---- 3. the value rows are read out with the current value ----
test('a value row label holds the name, the meaning and the current value (— when undefined)',()=>{
 const groups=[{kind:'obs',values:[{name:'obs.fps',text:'60.0'},{name:'obs.scene'}]}];
 const text=key=>({src_value_obs_fps:'fps',src_value_obs_scene:'scene',src_values_obs:'OBS'}[key]||key);
 const rows=sourceValueGroups(groups,'',key=>key.replace(/\./g,'_')).flatMap(g=>g.values);
 assert.equal(rows[0].label,'obs.fps: src_value_obs_fps: 60.0');
 assert.equal(rows[1].label,'obs.scene: src_value_obs_scene: —');
 assert.ok(text);
});

// ---- 5. "reload" with a draft asks first ----
test('reload asks only when the draft has a change, and cancel does nothing',()=>{
 const asked=[];
 const ask=answer=>message=>{asked.push(message);return answer;};
 const clean={data:{},values:{a:1},baseline:JSON.stringify({a:1})},changed={data:{},values:{a:2},baseline:JSON.stringify({a:1})};
 assert.equal(confirmReload(clean,ask(false),t),true);
 assert.deepEqual(asked,[]);
 assert.equal(confirmReload(changed,ask(false),t),false);
 assert.deepEqual(asked,['T:settings_discard']);
 assert.equal(confirmReload(changed,ask(true),t),true);
 assert.equal(locale('ja').settings_discard,locale('ja').src_discard);
 assert.equal(locale('en').settings_discard,locale('en').src_discard);
});

test('the reload button of the settings page goes through confirmReload',()=>{
 assert.match(read('settings.js'),/a==='reload'\)\{if\(!await confirmReload\(store\.get\(\),\(\)=>askConfirm\(/);
});
