// S2-86: the fixes after the Windows check S2-82 (spec v0.32).
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,readdirSync} from 'node:fs';
import {createStore} from '../assets/app/store.js';
import {createAPI,networkError} from '../assets/app/api.js';
import {isOnline,reconnected,setNotice,clearNotice,clearNetworkNotice,applyKey,reloadAfterApply,createSenderLoader} from '../assets/app/connection.js';
import {failureState,nothingToSave,noChangesNotice,finishAccept} from '../assets/app/settings-flow.js';
import {confirmationLines} from '../assets/app/settings-state.js';
import {streamingChanged,testNoticeFor,applySourceEvent,testInitial} from '../assets/app/sources-state.js';
import {cssContext,bodyAssetInsertion,assetInsertText} from '../assets/app/source-body.js';
import {styleStart,insideStyle} from '../assets/app/template-lexer.js';

const read=name=>readFileSync(new URL('../assets/app/'+name,import.meta.url),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8'));
const table={ja:locale('ja').ui,en:locale('en').ui};
const t=key=>'T:'+key;
const api=fetcher=>createAPI({lang:'ja',t,login:false},fetcher);

// ---- 1. the failed connection after a restart ----
test('a failed fetch is the text-table sentence, not the browser English',async()=>{
 const request=api(async()=>{throw new TypeError('Failed to fetch');});
 await assert.rejects(request('settings'),e=>{
  assert.equal(e.message,'T:disconnected');assert.equal(e.network,true);assert.equal(e.code,'api.unreachable');
  assert.ok(!/fetch/i.test(e.message));return true;
 });
 // Another kind of error is not turned into one.
 const other=api(async()=>{throw new RangeError('x');});
 await assert.rejects(other('settings'),e=>e instanceof RangeError&&!e.network);
 assert.equal(networkError(t).message,'T:disconnected');
 // The sentence exists in both languages.
 for(const lang of ['ja','en'])assert.ok(table[lang].disconnected);
});

test('failure without field errors does not throw and still shows a notice (409, 403, a lost connection)',()=>{
 for(const e of [Object.assign(Error('conflict'),{status:409}),Object.assign(Error('forbidden'),{status:403}),networkError(t)]){
  const s={data:null,errors:{},notice:'',group:'admin'};
  let target;assert.doesNotThrow(()=>{target=failureState(s,e,t,'settings');});
  assert.equal(target,'');
  assert.equal(s.notice,'T:settings_not_saved '+e.message);
  assert.deepEqual(s.errors,{});assert.equal(s.group,'admin');
  assert.equal(s.noticeNetwork,!!e.network);
 }
 // Without the field locations (a settings load that never finished) it also holds.
 assert.doesNotThrow(()=>failureState({data:{},errors:{}},Object.assign(Error('x'),{fields:{'cameras[cam1].name':'bad'}}),t,'cameras'));
});

test('failure with field errors moves to the page that holds the field, and only then',()=>{
 const data={field_locations:{'cameras[].name':{page:'cameras',group:'camera'},'log.keep_days':{page:'settings',group:'other'}}};
 const wrong=path=>Object.assign(Error('bad'),{fields:{[path]:'bad'}});
 let s={data,errors:{},group:'admin'};
 assert.equal(failureState(s,wrong('cameras[cam2].name'),t,'settings'),'cameras');
 assert.equal(s.group,'camera');
 s={data,errors:{},group:'admin'};
 assert.equal(failureState(s,wrong('log.keep_days'),t,'settings'),'');
 assert.equal(s.group,'other');
});

test('the failure path of the page no longer reads the first error key without a check',()=>{
 for(const name of ['settings.js','settings-flow.js'])assert.ok(!/Object\.keys\(s\.errors\)\[0\]\.replace/.test(read(name)),name);
});

test('restarting to apply neither reads nor announces a refresh; the read comes when the status says applied',async()=>{
 const events=[],loads=[],updates=[];
 const common={values:{a:1},preview:{},updateOBS:false,t,page:'cameras',update:fn=>{const s={baseline:'',notice:'x'};fn(s);updates.push(s);},dispatch:(name,detail)=>events.push([name,detail]),load:async()=>{loads.push(1);},closeDialog:()=>events.push(['close'])};
 const went=[];
 assert.equal(await finishAccept({...common,action:'save',result:{ok:true,apply:'restart',admin_url:'http://h:1/'},go:u=>went.push(u)}),'restart');
 assert.deepEqual(events,[['close'],['keeper-restarting',undefined]]);
 assert.equal(loads.length,0);assert.deepEqual(went,['http://h:1/']);
 assert.equal(updates[0].baseline,JSON.stringify({a:1}));assert.equal(updates[0].notice,'T:settings_saved');
 // The restart button itself, and a regenerate that needs a restart, are the same.
 events.length=0;
 await finishAccept({...common,action:'restart',result:{ok:true}});
 await finishAccept({...common,action:'regenerate',result:{ok:true,apply:'restart'}});
 assert.ok(events.every(e=>e[0]!=='keeper-settings-refresh'));assert.equal(loads.length,0);
 // Immediate apply reads now, as before.
 events.length=0;
 assert.equal(await finishAccept({...common,action:'save',result:{ok:true,apply:'immediate'}}),'applied');
 assert.deepEqual(events.map(e=>e[0]),['close','keeper-settings-refresh']);assert.equal(events[1][1],'cameras');
 assert.equal(loads.length,1);assert.equal(updates.at(-1).notice,'T:settings_applied');
});

test('the destinations are not read while restarting, and are read again when the status applied or reverted',async()=>{
 const store=createStore({restarting:true,host:'',sender:null,notice:'',noticeNetwork:false});
 const calls=[];
 const load=createSenderLoader({request:async p=>{calls.push(p);return {addresses:[{address:'a'}],default:'a'};},get:store.get,update:store.update,notice:e=>store.update(s=>setNotice(s,e))});
 assert.equal(await load(),false);assert.deepEqual(calls,[]);
 store.update(s=>{s.restarting=false;});
 assert.equal(await load(),true);assert.deepEqual(calls,['sender']);assert.equal(store.get().host,'a');
 // A failure shows the notice (it came from a lost connection, so it is marked as one).
 const failing=createSenderLoader({request:async()=>{throw networkError(t);},get:store.get,update:store.update,notice:e=>store.update(s=>setNotice(s,e))});
 await failing();
 assert.equal(store.get().notice,'T:disconnected');assert.equal(store.get().noticeNetwork,true);
 const status=state=>({generation:2,apply:{state}});
 const before=applyKey({generation:1,apply:{state:'applying',kind:'restart'}});
 assert.equal(reloadAfterApply(before,applyKey(status('applied')),status('applied')),true);
 assert.equal(reloadAfterApply(before,applyKey(status('reverted')),status('reverted')),true);
 assert.equal(reloadAfterApply(before,applyKey(status('failed')),status('failed')),false);
 assert.equal(reloadAfterApply(undefined,applyKey(status('applied')),status('applied')),false);
 assert.equal(reloadAfterApply(applyKey(status('applied')),applyKey(status('applied')),status('applied')),false);
});

test('coming back online removes the notices that a lost connection made, and only those',()=>{
 const offline={disconnected:true,restarting:false},restarting={disconnected:false,restarting:true},online={disconnected:false,restarting:false};
 assert.equal(isOnline(online),true);assert.equal(isOnline(offline),false);assert.equal(isOnline(restarting),false);
 assert.equal(reconnected(offline,online),true);assert.equal(reconnected(restarting,online),true);
 assert.equal(reconnected(online,online),false);assert.equal(reconnected(online,offline),false);
 const net={notice:'',noticeNetwork:false};setNotice(net,networkError(t));
 assert.equal(net.notice,'T:disconnected');clearNetworkNotice(net);assert.deepEqual(net,{notice:'',noticeNetwork:false});
 const other={notice:'',noticeNetwork:false};setNotice(other,Error('saved'));clearNetworkNotice(other);assert.equal(other.notice,'saved');
 clearNotice(other);assert.equal(other.notice,'');
});

// ---- 2. when a notice goes ----
test('the release notice of the OBS test stays only while streaming, and does not come back after a stop',()=>{
 const s={...testInitial(),streaming:true,test:true,testReason:'',testNotice:''};
 assert.equal(applySourceEvent(s,{sources:{},test_obs:{enabled:false,reason:'stream_started'}},'released'),true);
 assert.equal(testNoticeFor(s),'released');
 streamingChanged(s,false);
 assert.equal(testNoticeFor(s),'');assert.equal(s.testNotice,'');
 // The server keeps the reason after the stream ends; reading the page again must not show the notice again.
 const reload={...testInitial(),streaming:false,test:false,testReason:'',testNotice:''};
 applySourceEvent(reload,{sources:{},test_obs:{enabled:false,reason:'stream_started'}},'released');
 assert.equal(testNoticeFor(reload),'');
 // Changing how the test is chosen removes it (the page clears testNotice with the choice).
 const source=read('sources.js');
 assert.ok(/choose=\(axis,value\)=>\{change\(x=>\{x\[axis\]=value;x\.testNotice='';\}\)/.test(source));
 assert.ok(source.includes('testNoticeFor(s)'));
});

test('a notice goes when the same field is worked on again, or the page is left',()=>{
 const settings=read('settings.js');
 // an edit (and with it adding or removing a camera) clears "読み直しました。"
 assert.ok(/function set\(path,value\)\{update\(s=>\{clearNotice\(s\);/.test(settings));
 // reload, preview (save) start by clearing it
 assert.ok(settings.includes("if(a==='reload'){update(clearNotice);"));
 assert.ok(/async function preview\(action,payload\)\{[^]*?update\(clearNotice\);/.test(settings));
 // leaving the page, and coming back online
 assert.ok(settings.includes("if(e.detail!==page)update(clearNotice);"));
 assert.ok(settings.includes("keeper-reconnected"));
 const main=read('main.js');
 assert.ok(/'keeper-page',e=>\{update\(s=>\{clearNotice\(s\);s\.senderNotice='';\}\);/.test(main));
 assert.ok(main.includes("keeper-reconnected"));
 const sources=read('sources.js');
 assert.ok(sources.includes("s.editorNotice='';s.editorError='';fn(s)"));
 assert.ok(sources.includes("if(!active){s.notice='';s.editorNotice='';s.testNotice='';}"));
});

test('the status of a failed or reverted apply stops being shown when the state moves on',()=>{
 const settings=read('settings.js');
 assert.ok(/s\.applyNotice=\['failed','reverted'\]\.includes\(status\.apply\?\.state\)\?[^;]*:'';/.test(settings));
});

// ---- 3. a save with nothing changed ----
test('save with no change opens no window and says so; a change or another action goes on',()=>{
 const same={data:{},values:{a:1},baseline:JSON.stringify({a:1})},changed={data:{},values:{a:2},baseline:JSON.stringify({a:1})};
 assert.equal(nothingToSave('save',same),true);
 assert.equal(nothingToSave('save',changed),false);
 assert.equal(nothingToSave('restart',same),false);
 assert.equal(nothingToSave('regenerate',same),false);
 assert.equal(nothingToSave('save',{...same,data:null}),false);
 const s={notice:'old',noticeNetwork:true};noChangesNotice(s,t);
 assert.deepEqual(s,{notice:'T:settings_no_changes',noticeNetwork:false});
 assert.equal(table.ja.settings_no_changes,'変更はありません。');assert.ok(table.en.settings_no_changes);
 // The check comes before the request to preview (both pages share settings.js).
 const settings=read('settings.js');
 const check=settings.indexOf('nothingToSave(action,store.get())'),request=settings.indexOf("request('settings/preview'");
 assert.ok(check>0&&request>check);
});

// ---- 4. no undo on the admin page ----
test('the admin page has no undo: button, request, text and confirmation line are gone',()=>{
 for(const name of readdirSync(new URL('../assets/app/',import.meta.url)).filter(n=>n.endsWith('.js'))){
  const source=read(name);
  assert.ok(!/settings\/revert|'revert'|"revert"|file_hash|revert_credentials|settings_undone|settings_revert['"]/.test(source),name);
 }
 const lines=confirmationLines('save',{},{apply:'restart',fields:[]},false,k=>k,{});
 assert.ok(!lines.includes('revert_credentials'));
 for(const lang of ['ja','en'])for(const key of ['settings_revert','settings_undone','revert_credentials','sender'])assert.ok(!(key in table[lang]),lang+' '+key);
 // "settings_reverted" (the apply fell back to the old settings) stays.
 for(const lang of ['ja','en'])assert.ok(table[lang].settings_reverted);
});

// ---- 5. dropping an image into <style> ----
const url='url("/source/files/a.png")',rule='.image { background: url("/source/files/a.png") no-repeat; }';
const drop=(doc,cursor=doc.length)=>bodyAssetInsertion(doc,{from:cursor,to:cursor},true,'a.png').insert;
test('inside a declaration the drop is the url alone',()=>{
 for(const doc of ['<style>.a { background: ','<style>.a { ','<style>.a{color:red;','<style>.a { color: red; } .b { x:','<style>@media (min-width: 1px) { .a { ','<style>@font-face { src: ','<style>@keyframes k { from { ','<style>@keyframes k { 50% { background:','<style>@media screen { @supports (x:y) { .a { ','<style>.a { /* } */ background:'])assert.equal(drop(doc),url,doc);
});
test('outside rules the drop is a rule that shows the image',()=>{
 for(const doc of ['<style>','<style>\n','<style>.a { color: red; }','<style>.a { color: red; }\n','<style>.a { } .b { }','<style>@media (min-width: 1px) { ','<style>@media (min-width: 1px) { .a { x:y } ','<style>@media (min-width: 1px) { .a { x:y } }','<style>@supports (display:grid) { ','<style>.a','<style>/* a */','<style>@import "x.css";','<style>@media (max-width:1px) { } ','<style>.x[title="{"] { } '])assert.equal(drop(doc),rule,doc);
 assert.equal(assetInsertText('/p','rule'),'.image { background: url("/p") no-repeat; }');
});
test('directly in @keyframes, in a comment or in a string the drop is the url alone',()=>{
 for(const doc of ['<style>@keyframes k { ','<style>@keyframes k { from { x:y } ','<style>@-webkit-keyframes k { ','<style>/* ','<style>.a { /* ','<style>.a { content: "','<style>.a[title=\'x','<style>.a { content: "a\\" ','<style>@media x { /* '])assert.equal(drop(doc),url,doc);
});
test('outside <style> and in a not yet closed style the drop keeps its form; the cursor decides, not the end of the text',()=>{
 assert.equal(drop('<span>'),'<img src="/source/files/a.png">');
 assert.equal(drop('<style>.a{}</style><span>'),'<img src="/source/files/a.png">');
 assert.equal(drop('<!-- <style> -->'),'<img src="/source/files/a.png">');
 assert.equal(drop('<style>.a { color: red; }</style>',20),url);
 assert.equal(drop('<style>.a { color: red; }</style>',7),rule);
 assert.equal(drop('<style>.a { color: red; }</style>',25),rule);assert.equal(drop('<style>.a { color: red; }</style>',24),url);
 // The first style closed, a second one opened.
 assert.equal(drop('<style>.a{}</style><p><style>'),rule);
 assert.equal(styleStart('<span><style class="x">.a'),'<span><style class="x">'.length);
 assert.equal(styleStart('<span>'),-1);assert.equal(insideStyle('<style>x'),true);assert.equal(insideStyle('<style>x</style>'),false);
 // No cursor yet: the end of the text, with the same decision; nested paths stay encoded.
 assert.equal(bodyAssetInsertion('<style>.a { ',{from:0,to:0},false,'d/あ b.png').insert,'url("/source/files/d/%E3%81%82%20b.png")');
 // A selection is replaced.
 const r=bodyAssetInsertion('<style>XY',{from:7,to:9},true,'a.png');assert.deepEqual([r.from,r.to,r.insert],[7,9,rule]);
 assert.equal(cssContext(''),'rule');
});

// ---- 6. small fixes ----
test('the id of the admin page document is not repeated',()=>{
 const dir=new URL('../assets/app/',import.meta.url);
 const ids=new Map();
 for(const name of ['../page.html',...readdirSync(dir).filter(n=>n.endsWith('.js')).map(n=>'app/'+n)]){
  const text=readFileSync(new URL('../assets/'+name.replace('../',''),import.meta.url),'utf8');
  for(const m of text.matchAll(/\bid="([^"$]+)"/g)){
   (ids.get(m[1])||ids.set(m[1],[]).get(m[1])).push(name);
  }
 }
 // Two places in one template branch would be a repeat; the same id in the two branches of one conditional is not (the page renders one of them).
 const repeated=[...ids].filter(([,places])=>places.length>1);
 assert.deepEqual(repeated.map(([id,places])=>id+': '+places.join(',')),[]);
 const shell=read('shell.js');
 assert.equal((shell.match(/id="cameras-confirm"/g)||[]).length,1);
 assert.equal((shell.match(/id="settings-confirm"/g)||[]).length,1);
});

test('"Send to" button on the status page names the destination and goes to the camera card',()=>{
 const main=read('main.js');
 assert.ok(main.includes("@click=${()=>goSender(c.id)}>${t('sender_link')}"));
 assert.equal(table.ja.sender_link,'送り先');assert.equal(table.en.sender_link,table.en.camera_destination);
 // goSender: set the camera, open the Cameras page, scroll to its card (drawDestinations scrolls when the card is drawn).
 assert.ok(/function goSender\(id\)\{update\(s=>\{s\.camera=id;\}\);document\.querySelector\('\[data-page="cameras"\]'\)\.click\(\);/.test(main));
 assert.ok(main.includes("selected.scrollIntoView({block:'start'});update(x=>{x.camera='';});"));
 // The notice that sends the user to the destinations uses the new name.
 assert.ok(!table.ja.settings_pending_urls.includes('送信の設定'));
});

test('the Japanese warning of a missing media source has no space between the name and "の"',()=>{
 const ja=locale('ja').messages,en=locale('en').messages;
 assert.equal(ja['obs.source_missing'],'{name}のメディアソースが OBS に見つかりません');
 assert.equal(en['obs.source_missing'],'The media source for {name} is missing in OBS.');
});

test('warnings are shown one to a line on the destination area',()=>{
 const main=read('main.js');
 assert.ok(!main.includes(".join(' ')"));
 assert.ok(main.includes("].map(m=>html`<p>${m}</p>`)"));
});

test('narrow fields and button text: styles',()=>{
 const css=readFileSync(new URL('../assets/style.css',import.meta.url),'utf8');
 assert.ok(css.includes('.settings-field>span:not(.password-control){display:flex;flex:1 1 100%;min-width:0}'));
 assert.ok(css.includes('>input:not([type=checkbox]){flex:1;width:100%;min-width:0;box-sizing:border-box}'));
 assert.ok(css.includes('.btn,.seg button,#settings-groups button{word-break:keep-all;overflow-wrap:anywhere}'));
 assert.ok(css.includes('@supports (word-break:auto-phrase)'));
});

test('the source preview frame has its own name, not the name of the button',()=>{
 const source=read('sources.js');
 assert.ok(source.includes("frame.title=t('src_preview_frame')"));
 assert.ok(!source.includes("frame.title=t('src_previewonly')"));
 assert.equal(table.ja.src_preview_frame,'ソースのプレビュー');assert.equal(table.en.src_preview_frame,'Source preview');
 for(const lang of ['ja','en'])assert.notEqual(table[lang].src_preview_frame,table[lang].src_previewonly);
});
