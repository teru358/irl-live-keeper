// S2-90b: the fixes after the review of the admin page JS (breaks 4 and 5).
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,readdirSync} from 'node:fs';
import {createAPI} from '../assets/app/api.js';
import {connectEvents,openStream} from '../assets/app/events.js';
import {createStore} from '../assets/app/store.js';
import {copyValue} from '../assets/app/clipboard.js';
import {loadRules} from '../assets/app/rules.js';
import {simulateOf} from '../assets/app/sources-state.js';
import {confirmationLines} from '../assets/app/settings-state.js';
import {normalize} from '../assets/app/settings-pages.js';
import {applyStatus,sceneReadNeeded,restartBlocked,restartDirtyNotice,finishAccept,failureState,receiveErrors,applyLoaded} from '../assets/app/settings-flow.js';

const dir=new URL('../assets/app/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const readAsset=name=>readFileSync(new URL('../assets/'+name,import.meta.url),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8'));
const ui={ja:locale('ja').ui,en:locale('en').ui};
const t=key=>'T:'+key;
const sources=readdirSync(dir).filter(n=>n.endsWith('.js')).map(n=>[n,read(n)]);

// ---- Important 1: the scene choice of the settings page needs the OBS flag ----
test('a status event sets the OBS flag, and the scenes are read when OBS became connected and the page is loaded',()=>{
 const s={cameraStatus:null,streaming:false,obs:false,applyNotice:'x',data:{}};
 applyStatus(s,{cameras:[{id:'cam1',kbps:5}],obs:{connected:true,streaming:true},apply:{state:'failed',reason:'r'}},t);
 assert.equal(s.obs,true);assert.equal(s.streaming,true);assert.equal(s.cameraStatus.cam1.kbps,5);
 assert.equal(s.applyNotice,'T:settings_failed r');
 assert.equal(sceneReadNeeded(false,s),true);
 assert.equal(sceneReadNeeded(true,s),false);
 assert.equal(sceneReadNeeded(false,{...s,data:null}),false);
 applyStatus(s,{cameras:[],obs:{connected:false}},t);
 assert.equal(s.obs,false);assert.equal(s.applyNotice,'');assert.equal(sceneReadNeeded(false,s),false);
});

test('the status handler of the settings page uses them and reads the scenes',()=>{
 const settings=read('settings.js');
 assert.match(settings,/'keeper-status',e=>\{[^\n]*applyStatus\(s,status,t\)[^\n]*sceneReadNeeded\([^\n]*loadScenes\(\)/);
 assert.match(settings,/async function loadScenes\(\)\{[^\n]*request\('obs\/scenes'\)/);
});

// ---- Important 2: a restart with a draft ----
test('"restart" is refused while the page has a draft, and says so in both languages',()=>{
 const clean={data:{},values:{a:1},baseline:JSON.stringify({a:1})},changed={...clean,values:{a:2}};
 assert.equal(restartBlocked('restart',changed),true);
 assert.equal(restartBlocked('restart',clean),false);
 assert.equal(restartBlocked('save',changed),false);
 assert.equal(restartBlocked('restart',{...changed,data:null}),false);
 const s={notice:'old',noticeNetwork:true};restartDirtyNotice(s,t);
 assert.deepEqual(s,{notice:'T:settings_restart_dirty',noticeNetwork:false});
 assert.equal(ui.ja.settings_restart_dirty,'保存していない変更があります。保存するか「読み直す」で捨ててから再起動してください。');
 assert.equal(ui.en.settings_restart_dirty,'There are unsaved changes. Save them or discard them with "Reload" before restarting.');
 const settings=read('settings.js');
 const check=settings.indexOf('restartBlocked(action,store.get())'),request=settings.indexOf("request('settings/preview'");
 assert.ok(check>0&&request>check);
});

test('finishAccept keeps the baseline for a restart and moves it for a save',async()=>{
 const run=async action=>{
  const s={baseline:'saved',notice:''};
  await finishAccept({action,result:{apply:'restart'},values:{a:2},preview:{},updateOBS:false,t,update:fn=>fn(s),dispatch:()=>{},load:async()=>{},closeDialog:()=>{},page:'settings'});
  return s.baseline;
 };
 assert.equal(await run('restart'),'saved');
 assert.equal(await run('save'),JSON.stringify({a:2}));
});

test('the confirmation of a restart only does not say that the settings are applied',()=>{
 assert.deepEqual(confirmationLines('restart',{},{apply:'restart'},false,t),['T:settings_restart_only']);
 assert.deepEqual(confirmationLines('restart',{},{apply:'restart'},true,t),['T:settings_restart_only','T:settings_stream_warning']);
 assert.equal(confirmationLines('save',{},{apply:'restart',fields:[]},false,t)[0],'T:settings_restart_mode');
 assert.equal(ui.ja.settings_restart_only,'本体を再起動します（設定は変わりません）');
 assert.equal(ui.en.settings_restart_only,'The application restarts (settings do not change)');
});

// ---- Important 3: the event stream after the process was started again ----
function fakes(){
 const made=[],timers=[];
 class Source{constructor(url){this.url=url;this.readyState=1;this.listeners={};made.push(this);}addEventListener(type,f){this.listeners[type]=f;}close(){this.readyState=2;this.closed=true;}}
 return {made,timers,Source,set:(f,ms)=>{const timer={f,ms,cleared:false};timers.push(timer);return timer;},clear:timer=>{if(timer)timer.cleared=true;}};
}
const options=(f,probe)=>({probe,delay:3000,timers:{set:f.set,clear:f.clear}});

test('a closed event stream is opened again once an ordinary request works',async()=>{
 const f=fakes();let probes=0;
 const store=createStore({disconnected:false});
 connectEvents(store,'ja',f.Source,options(f,async()=>{probes++;}));
 assert.equal(f.made.length,1);
 f.made[0].readyState=2;f.made[0].onerror();
 assert.equal(store.get().disconnected,true);
 assert.equal(f.timers.length,1);assert.equal(f.timers[0].ms,3000);
 await f.timers[0].f();
 assert.equal(probes,1);assert.equal(f.made.length,2);assert.equal(f.made[1].url,'/api/events?topics=status&lang=ja');
 assert.ok(f.made[1].listeners.status);
});

test('a 401 from the probe stops the retries (the request already moved to the login), another failure tries again',async()=>{
 const f=fakes();
 openStream(()=>new f.Source(),options(f,async()=>{throw Object.assign(Error('x'),{status:401});}));
 f.made[0].readyState=2;f.made[0].onerror();
 await f.timers[0].f();
 assert.equal(f.made.length,1);assert.equal(f.timers.length,1);
 const g=fakes();
 openStream(()=>new g.Source(),options(g,async()=>{throw Object.assign(Error('net'),{network:true});}));
 g.made[0].readyState=2;g.made[0].onerror();
 await g.timers[0].f();
 assert.equal(g.made.length,1);assert.equal(g.timers.length,2);
});

test('a stream that the browser reconnects itself is left alone; close cancels the retry',async()=>{
 const f=fakes();let probes=0;
 const close=openStream(()=>new f.Source(),options(f,async()=>{probes++;}));
 f.made[0].readyState=0;f.made[0].onerror();
 assert.equal(f.timers.length,0);
 f.made[0].readyState=2;f.made[0].onerror();f.made[0].onerror();
 assert.equal(f.timers.length,1);
 close();
 assert.equal(f.timers[0].cleared,true);
 await f.timers[0].f();
 assert.equal(probes,0);assert.equal(f.made.length,1);
});

test('the status page and the sources page both open their streams with a status probe',()=>{
 assert.match(read('main.js'),/connectEvents\(store,lang,EventSource,\{probe:\(\)=>request\('status'\)\}\)/);
 const source=read('sources.js');
 assert.match(source,/openStream\(\(\)=>\{const source=new EventSource\('\/api\/events\?topics=sources/);
 assert.match(source,/probe:\(\)=>request\('status'\)/);
 assert.ok(!/events=new EventSource/.test(source));
});

// ---- Important 4: one copy part ----
function clipboardEnv({clipboard,exec=true}){
 const log=[];
 const area={style:{},focus(){log.push('focus');},select(){log.push('select');},setSelectionRange(){log.push('range');},remove(){log.push('remove');}};
 const document={createElement:()=>area,body:{append(){log.push('append');}},activeElement:{focus(){log.push('restore');}},execCommand:name=>{log.push('exec:'+name);if(exec instanceof Error)throw exec;return exec;}};
 return {navigator:{clipboard},document,log,area};
}
test('copyValue uses the clipboard API, then the execCommand fallback, and says whether it worked',async()=>{
 let written;
 const a=clipboardEnv({clipboard:{writeText:async v=>{written=v;}}});
 assert.equal(await copyValue('u',a.navigator,a.document),true);assert.equal(written,'u');assert.deepEqual(a.log,[]);
 const b=clipboardEnv({clipboard:{writeText:async()=>{throw new Error('denied');}}});
 assert.equal(await copyValue('v',b.navigator,b.document),true);
 assert.equal(b.area.value,'v');assert.ok(b.log.includes('exec:copy'));assert.ok(b.log.includes('remove'));
 const c=clipboardEnv({clipboard:undefined});
 assert.equal(await copyValue('w',c.navigator,c.document),true);
 const d=clipboardEnv({clipboard:undefined,exec:false});
 assert.equal(await copyValue('w',d.navigator,d.document),false);assert.ok(d.log.includes('remove'));
 const e=clipboardEnv({clipboard:undefined,exec:new Error('x')});
 assert.equal(await copyValue('w',e.navigator,e.document),false);
});
test('the sources page copies a URL through the same part as the destinations',()=>{
 const source=read('sources.js'),main=read('main.js');
 assert.match(source,/import \{copyValue\} from '\.\/clipboard\.js'/);
 assert.match(source,/await copyValue\(url\)/);
 assert.ok(!source.includes('navigator.clipboard'));
 assert.match(main,/import \{copyValue\} from '\.\/clipboard\.js'/);
 assert.ok(!/async function copyValue/.test(main));
});

// ---- Minor 5: an unreadable free JSON ----
test('an unreadable free JSON is the sentence of the text table, never the browser English',()=>{
 const state={line:'actual',standby:'actual',stream:'actual',free:true,custom:'{'};
 assert.throws(()=>simulateOf(state,t),e=>e.message==='T:src_custom_invalid');
 for(const custom of ['[1]','3','null'])assert.throws(()=>simulateOf({...state,custom},t),e=>e.message==='T:src_custom_invalid',custom);
 assert.deepEqual(simulateOf({...state,custom:'{"cam1.kbps":600}'},t),{values:{'cam1.kbps':600}});
 assert.deepEqual(simulateOf({...state,free:false,custom:'{'},t),{});
 assert.equal(ui.ja.src_custom_invalid,'自由の値を JSON のオブジェクトとして読めません。');assert.ok(ui.en.src_custom_invalid);
 assert.match(read('sources.js'),/const sim=s=>simulateOf\(s,t\)/);
});

// ---- Minor 6: a failed read has no "not saved" ----
test('a failure while reading the settings does not say "not saved"',()=>{
 const e=Error('x');
 const reading={data:null,errors:{},notice:''};failureState(reading,e,t,'settings',false);
 assert.equal(reading.notice,'x');
 const saving={data:null,errors:{},notice:''};failureState(saving,e,t,'settings');
 assert.equal(saving.notice,'T:settings_not_saved x');
 assert.match(read('settings.js'),/async function load\(background=false\)\{[^\n]*failure\(e,false\)/);
});

// ---- Minor 7: a field error of the other page goes to that page ----
test('a field error that belongs to the other page is handed to the store of that page',()=>{
 const data={field_locations:{'cameras[].name':{page:'cameras',group:'camera'}}};
 const s={data,errors:{},group:'common',notice:'',noticeNetwork:true,carryErrors:false};
 receiveErrors(s,{errors:{'cameras[cam2].name':'bad'},message:'T:settings_not_saved bad',network:false});
 assert.deepEqual(s.errors,{'cameras[cam2].name':'bad'});assert.equal(s.group,'camera');
 assert.equal(s.notice,'T:settings_not_saved bad');assert.equal(s.noticeNetwork,false);assert.equal(s.carryErrors,false);
 // The page has not read its settings yet: the read that the move starts must not erase them.
 const early={data:null,errors:{},group:'common',notice:'',carryErrors:false};
 receiveErrors(early,{errors:{'cameras[cam2].name':'bad'},message:'m'});
 assert.equal(early.carryErrors,true);
 const settings=read('settings.js');
 assert.ok(settings.includes("'keeper-field-errors'"));
 assert.ok(settings.indexOf("CustomEvent('keeper-field-errors'")<settings.indexOf("CustomEvent('keeper-navigate'"));
});

// ---- Minor 8: a background read keeps the draft written meanwhile ----
test('a background read puts the draft on the new saved values instead of giving up',()=>{
 const locations={'log.keep_days':{page:'settings',group:'other'},'log.dir':{page:'settings',group:'other'}};
 const data=hash=>({hash,values:{log:{keep_days:14,dir:'d'}},field_locations:locations});
 const state={data:data('old'),values:{log:{keep_days:7,dir:'d'}},baseline:JSON.stringify({log:{keep_days:14,dir:'d'}}),errors:{x:'e'},scenes:[],notice:''};
 const fresh=data('new');fresh.values.log.dir='e';
 applyLoaded(state,{data:fresh,scenes:['s'],page:'settings',background:true,t,normalize});
 assert.equal(state.data.hash,'new');
 assert.deepEqual(state.values,{log:{keep_days:7,dir:'e'}});
 assert.equal(state.baseline,JSON.stringify({log:{keep_days:14,dir:'e'}}));
 assert.deepEqual(state.scenes,['s']);assert.deepEqual(state.errors,{});
 // A read the user asked for replaces the draft.
 applyLoaded(state,{data:fresh,scenes:[],page:'settings',background:false,t,normalize});
 assert.deepEqual(state.values,{log:{keep_days:14,dir:'e'}});
 // Errors carried over from the other page stay once.
 const carried={...state,errors:{'log.dir':'bad'},carryErrors:true};
 applyLoaded(carried,{data:fresh,scenes:[],page:'settings',background:false,t,normalize});
 assert.deepEqual(carried.errors,{'log.dir':'bad'});assert.equal(carried.carryErrors,false);
 assert.ok(!/before!==JSON\.stringify/.test(read('settings.js')));
});

// ---- Minor 9: the HTML rules are read again after a failure ----
test('the HTML rules are asked again after a network error or a bad answer',async()=>{
 let attempts=0,waits=0;
 const fetcher=async()=>{attempts++;if(attempts===1)throw Error('offline');return {ok:attempts>2,json:async()=>({rules:true})};};
 assert.deepEqual(await loadRules(fetcher,async()=>{waits++;}),{rules:true});
 assert.equal(attempts,3);assert.equal(waits,2);
 assert.match(read('sources.js'),/const rulesPromise=loadRules\(\)/);
});

// ---- Minor 10: uploads go through request, so a 401 goes to the login ----
test('request sends a FormData as it is and moves to the login on 401',async()=>{
 let url,options,redirect;
 const request=createAPI({lang:'ja',t,login:false},async(u,o)=>{url=u;options=o;return {status:401,ok:false,json:async()=>({error:'unauthorized'})};},u=>redirect=u);
 const body=new FormData();body.append('file',new Blob(['x']),'a.png');
 await assert.rejects(request('source-files',body));
 assert.equal(url,'/api/source-files?lang=ja');assert.equal(options.method,'POST');assert.strictEqual(options.body,body);
 assert.equal(options.headers['X-IRL-Live-Keeper'],'1');assert.ok(!('Content-Type' in options.headers));
 assert.equal(redirect,'/?login');
 const json=createAPI({lang:'ja',t,login:false},async(u,o)=>{options=o;return {status:200,ok:true,json:async()=>({})};});
 await json('x',{a:1});assert.equal(options.headers['Content-Type'],'application/json');assert.equal(options.body,'{"a":1}');
 assert.ok(!/[^.\w]fetch\(/.test(read('sources.js')));
});

// ---- the things that nothing used ----
test('code that nothing in the page calls is gone',()=>{
 const gone=[/textareaInsertion/,/assetText/,/tokenizeTemplate/,/insideStyle/,/insertValue/,/insertCondition/,/getValue/,/settingsBusy/,/keeper-settings-lock/,/used_titles/,/kind==='css'/,/lang-css/,/-lan'\} hidden/,/for\(const type of \['status'\]\)/];
 for(const [name,text] of sources)for(const re of gone)assert.ok(!re.test(text),name+' '+re);
});
test('the password eye is one part',()=>{
 let count=0;for(const [,text] of sources)count+=(text.match(/M2 12s4-7/g)||[]).length;
 assert.equal(count,1);
 for(const name of ['shell.js','settings.js','main.js'])assert.match(read(name),/passwordEye\(/);
 // (eye.js imports lit-html, which needs a document: the part is read, not run, here.)
 assert.match(read('eye.js'),/export const passwordEye=\(\{label,pressed,toggle\}\)=>html`<button type="button" class="password-eye" aria-label=\$\{label\} aria-pressed=\$\{String\(!!pressed\)\} @click=\$\{toggle\}>/);
 assert.match(read('settings.js'),/export function mountSettings\(t,request,page='settings'\)/);
});
test('the settings of the own page are read once after a save',()=>{
 assert.match(read('settings.js'),/'keeper-settings-refresh',e=>\{if\(e\.detail===page\)return;load\(true\);\}/);
});
test('the unused styles are gone',()=>{
 const css=readAsset('style.css');
 for(const re of [/nav\.rail/,/\.cams\b/,/\.standby-tile/,/\.k-half/,/#sender-panel/,/#sender-address/,/#sender-regenerate/,/\.source-color/])assert.ok(!re.test(css),String(re));
 // Neighbours of the removed rules stay.
 assert.ok(css.includes('.sender-field input{'));assert.ok(css.includes('fieldset{border:1px solid var(--line)'));assert.ok(css.includes('.standby-banner'));
});
test('the unused sentences are gone from both tables, and the Japanese of the API errors is Japanese',()=>{
 const keys=['sources_duplicate','sources_invalid','sources_unreadable','sources_write_failed','src_value','studio','standby_on','standby_off','sender_hint','sender_port','sender_publish_host','setting_admin_help','setting_cameras_help','setting_gui_help','setting_log_help','setting_mediamtx_help','setting_obs_help','setting_stream_help','setting_switch_help','setting_on_stream_start_help','setting_id_help'];
 for(const lang of ['ja','en'])for(const key of keys)assert.ok(!(key in ui[lang]),lang+' '+key);
 assert.deepEqual(Object.keys(ui.ja).sort(),Object.keys(ui.en).sort());
 const ja=locale('ja');
 for(const key of ['api.not_found','api.method'])assert.match(ja.messages[key]??ja.ui[key],/[぀-ヿ一-鿿]/,key);
});

// ---- the whole-file hash is gone from GET /api/settings: the regenerate request does not send it ----
test('the body of a regenerate has no hash, and nothing reads the whole-file hash',()=>{
 const settings=read('settings.js');
 assert.match(settings,/preview\('regenerate',\{target,camera\}\)/);
 assert.ok(!/preview\('regenerate',\{[^}]*hash/.test(settings));
 for(const [name,text] of sources)assert.ok(!/data\.hash\b/.test(text),name);
 assert.ok(settings.includes('page_hashes[page]'));
});
