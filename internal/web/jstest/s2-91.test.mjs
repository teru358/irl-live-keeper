// S2-91: the confirmation window, the stream button, deleting a camera that leaves its card, and the texts of both.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,readdirSync} from 'node:fs';
import {confirmSpec,confirmContent,confirmIds,showConfirm,createAsk} from '../assets/app/confirm-dialog.js';
import {streamControl,startScene,streamConfirm} from '../assets/app/stream-control.js';
import {dirty,removeCamera,undoCameraRemoval,cameraCards,confirmationSpec,confirmationTitle} from '../assets/app/settings-state.js';

const dir=new URL('../assets/app/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
const ui={ja:locale('ja'),en:locale('en')};
const tja=key=>ui.ja[key];
const ten=key=>ui.en[key];

// ---- fakes: a lit-like html that keeps the functions, and a dialog that has no DOM ----
function fakeHTML(){
 const handlers=[];
 const text=v=>Array.isArray(v)?v.map(text).join(''):v&&typeof v==='object'?v.parts.map((p,i)=>p+(i<v.values.length?text(v.values[i]):'')).join(''):typeof v==='function'?'<fn#'+(handlers.push(v)-1)+'>':v===undefined||v===null?'':String(v);
 return {handlers,html:(parts,...values)=>({parts:[...parts],values}),text};
}
function fakeDialog(){
 const listeners={},yes={name:'yes',focused:0,focus(){this.focused++;}},no={name:'no',focused:0,focus(){this.focused++;}};
 const dialog={
  attributes:{},open:false,removed:false,yes,no,
  setAttribute(k,v){this.attributes[k]=v;},
  showModal(){this.open=true;},
  querySelector(selector){return selector==='[data-confirm="yes"]'?yes:selector==='[data-confirm="no"]'?no:null;},
  addEventListener(type,f){(listeners[type]||=[]).push(f);},
  removeEventListener(type,f){listeners[type]=(listeners[type]||[]).filter(x=>x!==f);},
  contains:()=>false,
  close(){if(!this.open)return;this.open=false;for(const f of [...(listeners.close||[])])f();},
  remove(){this.removed=true;},
 };
 return dialog;
}
const fakeOpener=(extra={})=>({isConnected:true,disabled:false,focused:0,focus(){this.focused++;},...extra});

// ---- the decision of a window ----
test('a window that ends or deletes is red and opens on 「いいえ」; the others are blue and open on 「はい」',()=>{
 const danger=confirmSpec(tja,{title:'配信を終了しますか？',lines:['OBS の配信を終了します。',''],danger:true});
 assert.deepEqual([danger.title,danger.lines,danger.yesClass,danger.focus,danger.yesLabel,danger.noLabel],['配信を終了しますか？',['OBS の配信を終了します。'],'btn stop','no','はい','いいえ']);
 const plain=confirmSpec(tja,{title:'設定を保存しますか？'});
 assert.deepEqual([plain.lines,plain.danger,plain.yesClass,plain.focus],[[],false,'btn primary','yes']);
 const en=confirmSpec(ten,{title:'Save the settings?'});
 assert.deepEqual([en.yesLabel,en.noLabel],['Yes','No']);
});

test('the content names the window by its title and wires 「はい」 and 「いいえ」',()=>{
 const f=fakeHTML(),spec=confirmSpec(tja,{title:'題ですか？',lines:['一行目','二行目'],danger:true});
 const calls=[];
 const out=f.text(confirmContent(f.html,spec,{ids:confirmIds('c'),onYes:()=>calls.push('yes'),onNo:()=>calls.push('no'),disabled:true}));
 assert.match(out,/<h2 id=c-title>題ですか？<\/h2>/);
 assert.match(out,/<div id=c-lines class=confirm-lines><p>一行目<\/p><p>二行目<\/p><\/div>/);
 assert.match(out,/<button id=c-yes type="button" class=btn stop confirm-yes data-confirm="yes" \?disabled=true @click=<fn#0>>はい<\/button>/);
 assert.match(out,/<button id=c-no type="button" class="btn confirm-no" data-confirm="no" \?disabled=true @click=<fn#1>>いいえ<\/button>/);
 f.handlers[0]();f.handlers[1]();assert.deepEqual(calls,['yes','no']);
});

// ---- opening, the first focus, Escape, and the way back to the opener ----
test('the first focus is 「いいえ」 for an ending operation and 「はい」 for the others',()=>{
 for(const [danger,focused] of [[true,'no'],[false,'yes']]){
  const dialog=fakeDialog();
  showConfirm(dialog,confirmSpec(tja,{title:'x',danger}),fakeOpener(),{doc:{},defer:fn=>fn()});
  assert.equal(dialog.open,true);
  assert.deepEqual([dialog.yes.focused,dialog.no.focused],focused==='yes'?[1,0]:[0,1]);
 }
});

test('closed by a button or by Escape, the focus goes back to the button that opened it, after the busy state',()=>{
 const dialog=fakeDialog(),opener=fakeOpener({disabled:true}),deferred=[];
 const doc={body:{},activeElement:null};
 showConfirm(dialog,confirmSpec(tja,{title:'x'}),opener,{doc,defer:fn=>deferred.push(fn)});
 dialog.close(); // Escape and 「いいえ」 both end here
 assert.equal(deferred.length,1);
 deferred.shift()(); // the opener is still disabled (the request that opened the window is running): it waits
 assert.equal(opener.focused,0);assert.equal(deferred.length,1);
 deferred.shift()();assert.equal(opener.focused,0);
 opener.disabled=false;deferred.shift()();
 assert.equal(opener.focused,1);assert.equal(deferred.length,0);
});

test('the focus does not go back when another element has taken it (a wrong field) or the opener is gone',()=>{
 const run=(opener,active)=>{
  const dialog=fakeDialog(),doc={body:{},activeElement:active};
  showConfirm(dialog,confirmSpec(tja,{title:'x'}),opener,{doc,defer:fn=>fn()});
  dialog.close();
  return opener.focused;
 };
 assert.equal(run(fakeOpener(),{field:true}),0);
 assert.equal(run(fakeOpener({isConnected:false}),null),0);
 assert.equal(run(fakeOpener(),null),1);
});

test('ask resolves true for 「はい」, false for 「いいえ」 and for Escape, and takes its window away',async()=>{
 const f=fakeHTML(),dialogs=[],body={items:[],append(d){this.items.push(d);}};
 const doc={body,activeElement:null,createElement:tag=>{assert.equal(tag,'dialog');const d=fakeDialog();dialogs.push(d);return d;}};
 const ask=createAsk({document:doc,html:f.html,render:(template,element)=>{element.rendered=f.text(template);},defer:fn=>fn()});
 const opener=fakeOpener();
 // 「はい」
 let answer=ask(confirmSpec(tja,{title:'保存しますか？'}),opener);
 const first=dialogs[0];
 assert.equal(first.attributes['aria-labelledby'],'confirm-1-title');
 assert.match(first.rendered,/<h2 id=confirm-1-title>保存しますか？<\/h2>/);
 assert.equal(first.open,true);
 f.handlers[0]();
 assert.equal(await answer,true);assert.equal(first.removed,true);
 // 「いいえ」
 f.handlers.length=0;
 answer=ask(confirmSpec(tja,{title:'削除しますか？',danger:true}),opener);
 assert.equal(dialogs[1].no.focused,1);
 f.handlers[1]();
 assert.equal(await answer,false);
 // Escape: the dialog closes by itself
 answer=ask(confirmSpec(tja,{title:'x'}),opener);
 dialogs[2].close();
 assert.equal(await answer,false);assert.equal(dialogs[2].removed,true);
 assert.ok(opener.focused>=3);
});

// ---- no browser confirm is left ----
test('no application script opens the browser confirm window, calls it or passes it on',()=>{
 for(const name of readdirSync(dir).filter(n=>n.endsWith('.js'))){
  const source=read(name);
  assert.doesNotMatch(source,/(^|[^\w.$'"?=])confirm\s*\(/,name+': confirm(');
  assert.doesNotMatch(source,/[(,]\s*confirm\s*[,)]/,name+': confirm passed on');
  assert.doesNotMatch(source,/window\.confirm|globalThis\.confirm/,name);
 }
});

test('every confirmation has the same form: a title that asks, 「はい」 and 「いいえ」',()=>{
 for(const lang of ['ja','en']){
  assert.equal(ui[lang].confirm_yes,lang==='ja'?'はい':'Yes');
  assert.equal(ui[lang].confirm_no,lang==='ja'?'いいえ':'No');
  const mark=lang==='ja'?'？':'?';
  for(const key of ['confirm_save','confirm_restart','confirm_regenerate_admin','confirm_regenerate_read','confirm_regenerate_publish','confirm_lan','show_all_confirm','stream_start_title','stream_stop_title','settings_discard','src_discard','src_confirmdelete'])
   assert.ok(ui[lang][key].endsWith(mark),lang+' '+key);
 }
 assert.deepEqual(['confirm_save','confirm_restart','confirm_regenerate_publish','stream_start_title','stream_stop_title'].map(tja),['設定を保存しますか？','本体を再起動しますか？','このカメラの送信用パスワードを作り直しますか？','配信を開始しますか？','配信を終了しますか？']);
 // the old browser-window texts and the old buttons are gone
 for(const lang of ['ja','en'])for(const key of ['confirm_stop','settings_confirm','settings_accept','settings_lan'])assert.ok(!(key in ui[lang]),lang+' '+key);
 assert.equal(ui.ja.settings_discard,ui.ja.src_discard);assert.equal(ui.en.settings_discard,ui.en.src_discard);
 assert.ok(!/破棄/.test(ui.ja.settings_discard));
 // the phone window is not a confirmation window and keeps its button text
 assert.ok(ui.ja.phone_close&&ui.en.phone_close&&!('settings_cancel' in ui.ja));
});

test('each window of the settings page is a confirmation window of the same form',()=>{
 const settings=read('settings.js'),shell=read('shell.js');
 assert.match(settings,/confirmContent\(html,spec,\{ids:\{title:page\+'-confirm-title',lines:page\+'-preview',yes:page\+'-accept',no:page\+'-cancel'\}/);
 assert.match(shell,/<dialog id="settings-confirm" class="confirm-dialog" aria-labelledby="settings-confirm-title">/);
 assert.match(shell,/<dialog id="cameras-confirm" class="confirm-dialog" aria-labelledby="cameras-confirm-title">/);
 // the focus goes back to the button that was pressed, which is read before the busy state disables it
 assert.match(settings,/onSave:e=>preview\('save',undefined,e\.currentTarget\)/);
 assert.match(settings,/onRestart:e=>preview\('restart',undefined,e\.currentTarget\)/);
 assert.match(settings,/showConfirm\(dialog,specOf\(store\.get\(\)\),opener\)/);
 assert.match(settings,/const opener=document\.activeElement;const target=/);
 // the LAN switch asks first and is turned on only by 「はい」
 assert.match(settings,/if\(path==='admin\.lan'&&e\.target\.checked\)\{const box=e\.target;box\.checked=false;askConfirm\([^]*?\.then\(ok=>\{if\(ok\)set\(path,true\);\}\);return;\}/);
});

test('the title and the colour of a settings window come from the operation',()=>{
 const t=k=>k;
 assert.equal(confirmationTitle('save',{},t),'confirm_save');
 assert.equal(confirmationTitle('restart',{},t),'confirm_restart');
 for(const target of ['admin','read','publish'])assert.equal(confirmationTitle('regenerate',{target},t),'confirm_regenerate_'+target);
 const saved={cameras:[{id:'cam1',name:''},{id:'cam2',name:'サブ'}]};
 // not streaming, saving, restarting and regenerating are blue, and open on 「はい」 (spec v0.37). While streaming, only an operation that restarts the app is red (S2-92)
 for(const [action,preview,streaming] of [['save',{apply:'restart',fields:[{path:'admin.port'}]},false],['restart',{apply:'restart'},false],['regenerate',{apply:'restart'},false],['save',{apply:'restart',fields:[{path:'cameras[cam3]',kind:'added'}]},false],['save',{apply:'immediate',fields:[{path:'log.keep_days'}]},true],['regenerate',{apply:'immediate'},true]]){
  const spec=confirmationSpec(action,{target:'publish'},preview,streaming,t,saved);
  assert.deepEqual([spec.danger,spec.yesClass,spec.focus],[false,'btn primary','yes'],action);
 }
 // a save that deletes a camera is red and opens on 「いいえ」
 const removing=confirmationSpec('save',{values:{cameras:[saved.cameras[0]]}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'}]},false,t,saved);
 assert.deepEqual([removing.title,removing.danger,removing.yesClass,removing.focus],['confirm_save',true,'btn stop','no']);
 assert.ok(removing.lines.includes('settings_camera 2 (サブ) — settings_removed')||removing.lines.some(l=>l.includes('settings_removed')));
 // the lines under the title keep the text of the old window
 assert.equal(removing.lines[0],'settings_restart_mode');
});

test('a save that adds or deletes a camera says that every camera loses its reception, also when nothing is streaming (A-4)',()=>{
 const t=k=>k,saved={cameras:[{id:'cam1'}]};
 const lines=(streaming,fields)=>confirmationSpec('save',{},{apply:'restart',fields},streaming,t,saved).lines;
 for(const kind of ['added','removed']){
  assert.ok(lines(false,[{path:'cameras[cam2]',kind}]).includes('settings_cameras_reset'),kind);
  // while streaming, the warning of the restart already says it
  assert.ok(!lines(true,[{path:'cameras[cam2]',kind}]).includes('settings_cameras_reset'),kind+' streaming');
 }
 assert.ok(!lines(false,[{path:'cameras[cam1].name'}]).includes('settings_cameras_reset'));
 assert.ok(!lines(false,[{path:'admin.port'}]).includes('settings_cameras_reset'));
 assert.equal(ui.ja.settings_cameras_reset,'全カメラの受信が一度切れます。');
 assert.equal(ui.ja.publish_reset_warning,'MediaMTX が起動し直すので、全カメラの受信が一度切れます。');
});

// ---- the stream button ----
const status=(connected,streaming)=>({obs:{connected,streaming}});
test('one button follows the stream state: 配信開始 (blue) or 配信終了 (red)',()=>{
 const start=streamControl(status(true,false),tja),stop=streamControl(status(true,true),tja);
 assert.deepEqual([start.path,start.label,start.cls,start.disabled,start.reason],['stream/start','配信開始','btn primary wide',false,'']);
 assert.deepEqual([stop.path,stop.label,stop.cls,stop.disabled,stop.reason],['stream/stop','配信終了','btn stop wide',false,'']);
 assert.equal(streamControl(status(true,false),ten).label,'Start streaming');
 assert.equal(streamControl(status(true,true),ten).label,'Stop streaming');
});

test('the stream button cannot be pressed while OBS is not connected, and says why',()=>{
 for(const [lang,t,reason] of [['ja',tja,'OBS につながっていません。'],['en',ten,'Not connected to OBS.']]){
  for(const s of [status(false,false),status(false,true),null,{}]){
   const c=streamControl(s,t);
   assert.equal(c.disabled,true,lang);assert.equal(c.reason,reason,lang);
  }
 }
 const main=read('main.js');
 assert.match(main,/<button id="stream-button" class=\$\{stream\.cls\} data-command=\$\{stream\.path\} \?disabled=\$\{stream\.disabled\|\|!!s\.busy\} aria-describedby=\$\{stream\.reason\?'stream-reason':nothing\}/);
 assert.match(main,/<p id="stream-reason" class="hint">/);
 // the two old buttons are gone
 assert.ok(!main.includes("button('stream/start'")&&!main.includes("button('stream/stop'"));
});

test('the confirmation of the stream button: the title asks, starting names the scene it switches to',()=>{
 const start=streamConfirm('stream/start',tja,'配信開始');
 // S2-94: the lines say what happens and do not say the title again
 assert.deepEqual([start.title,start.lines,start.danger,start.yesClass,start.focus],['配信を開始しますか？',['OBS は「配信開始」のシーンに切り替わります。'],false,'btn primary','yes']);
 assert.deepEqual(streamConfirm('stream/start',tja).lines,[]);
 const stop=streamConfirm('stream/stop',tja);
 assert.deepEqual([stop.title,stop.lines,stop.danger,stop.yesClass,stop.focus,stop.yesLabel,stop.noLabel],['配信を終了しますか？',['視聴者の映像が切れます。'],true,'btn stop','no','はい','いいえ']);
 assert.equal(streamConfirm('stream/start',ten,'Live').lines[0],'OBS will switch to the scene “Live”.');
 assert.equal(streamConfirm('stream/stop',ten).title,'Stop streaming?');
 // a scene name that has $& in it is put in as it is
 assert.equal(streamConfirm('stream/start',tja,'$&-$1').lines[0],'OBS は「$&-$1」のシーンに切り替わります。');
});

test('the scene of on_stream_start comes from the settings values, only when it is switched on',()=>{
 assert.equal(startScene({values:{on_stream_start:{enabled:true,scene:'Live'}}}),'Live');
 assert.equal(startScene({values:{on_stream_start:{enabled:false,scene:'Live'}}}),'');
 assert.equal(startScene({values:{on_stream_start:{enabled:true,scene:''}}}),'');
 assert.equal(startScene({values:{}}),'');assert.equal(startScene(null),'');
});

test('the stream button asks before it acts, reads the settings only to start, and does not act when the state changed meanwhile',()=>{
 const main=read('main.js');
 const body=main.slice(main.indexOf('async function streamToggle('),main.indexOf('const loadSender='));
 assert.match(body,/if\(control\.path==='stream\/start'\)\{try\{scene=startScene\(await request\('settings'\)\);\}catch\(e\)\{notice\(e,'ops'\);return;\}\}/);
 assert.ok(body.indexOf('askConfirm(streamConfirm(')>body.indexOf('startScene(')&&body.indexOf('await command(control.path)')>body.indexOf('askConfirm('));
 assert.match(body,/now\.disabled\|\|now\.path!==control\.path/);
 // the other command that asks: back to showing all camera sources
 assert.match(main,/path==='cameras\/show-all'&&!await askConfirm\(confirmSpec\(t,\{title:t\('show_all_confirm'\),lines:\[t\('confirm_show'\)\]\}\),opener\)/);
});

// ---- deleting a camera leaves its card ----
const cam=(id,name='')=>({id,name,on_bad:'standby',switch:{fail_kbps:null}});
const page=(...ids)=>{
 const values={cameras:ids.map(id=>cam(id,id+' name'))};
 return {data:{values:{cameras:ids.map(id=>cam(id,id+' name'))}},values,baseline:JSON.stringify(values),removedCameraIDs:[],removedRows:{}};
};
test('deleting a saved camera takes it out of the draft and keeps a card that says 保存すると削除します, in the order of the ids',()=>{
 const s=page('cam1','cam2','cam3');
 assert.equal(dirty(s),false);
 assert.equal(removeCamera(s,'cam2'),true);
 assert.deepEqual(s.values.cameras.map(c=>c.id),['cam1','cam3']);
 assert.equal(dirty(s),true);
 assert.deepEqual(s.removedCameraIDs,['cam2']);
 const cards=cameraCards(s.values.cameras,s.data.values.cameras,s.removedCameraIDs,s.removedRows);
 assert.deepEqual(cards.map(c=>[c.id,c.removing]),[['cam1',false],['cam2',true],['cam3',false]]);
 assert.equal(cards[1].row.name,'cam2 name');
});

test('取り消す puts the camera back with what was edited in it, and the draft is clean again',()=>{
 const s=page('cam1','cam2');
 s.values.cameras[1].name='直した';s.baseline=JSON.stringify(page('cam1','cam2').values);
 assert.equal(dirty(s),true);
 removeCamera(s,'cam2');
 assert.equal(undoCameraRemoval(s,'cam2'),true);
 assert.deepEqual(s.values.cameras.map(c=>[c.id,c.name]),[['cam1','cam1 name'],['cam2','直した']]);
 assert.deepEqual([s.removedCameraIDs,s.removedRows],[[],{}]);
 // without edits the draft is the saved state again
 const t=page('cam1','cam2');removeCamera(t,'cam1');assert.equal(dirty(t),true);undoCameraRemoval(t,'cam1');
 assert.equal(dirty(t),false);assert.deepEqual(t.values.cameras.map(c=>c.id),['cam1','cam2']);
 // nothing to undo for a camera that is in the draft
 assert.equal(undoCameraRemoval(t,'cam1'),false);
});

test('a camera that was added in the draft and deleted leaves no card (nothing is saved to delete), and its id is not given again',()=>{
 const s=page('cam1');
 s.values.cameras.push(cam('cam2'));
 removeCamera(s,'cam2');
 assert.deepEqual(s.values.cameras.map(c=>c.id),['cam1']);
 assert.deepEqual(cameraCards(s.values.cameras,s.data.values.cameras,s.removedCameraIDs,s.removedRows).map(c=>c.id),['cam1']);
 assert.deepEqual(s.removedCameraIDs,['cam2']);
 assert.equal(undoCameraRemoval(s,'cam2'),false);
 assert.equal(removeCamera(s,'cam9'),false);
});

test('the camera card: a red 削除 that names its camera, and a pending card without fields',()=>{
 const settings=read('settings.js');
 assert.match(settings,/<button type="button" class="btn stop" data-card-action="remove" aria-label=\$\{t\('settings_remove_camera'\)\.replace\('\{camera\}',\(\)=>cameraName\(v\)\)\} @click=\$\{\(\)=>remove\(i\)\}>\$\{t\('settings_remove'\)\}<\/button>/);
 const pending=settings.slice(settings.indexOf('const pendingCard='),settings.indexOf('\n',settings.indexOf('const pendingCard=')));
 assert.match(pending,/class="warn removing-note">\$\{t\('settings_remove_pending'\)\}/);
 assert.match(pending,/data-card-action="undo" aria-label=\$\{t\('settings_remove_undo_camera'\)\.replace\('\{camera\}',\(\)=>cameraName\(row\)\)\} @click=\$\{\(\)=>undoRemove\(row\.id\)\}>\$\{t\('settings_remove_undo'\)\}/);
 // the pending card has no field that is bound to the draft (the draft has no such camera)
 assert.ok(!pending.includes('field(s,'));
 assert.match(settings,/removeCamera\(x,id\)/);
 // other rows that are deleted (the scene fallbacks) are red too
 assert.match(settings,/<fieldset class="settings-card"><legend>[^]*?<button type="button" class="btn stop" @click=\$\{\(\)=>remove\(i\)\}>/);
 const names=(t,camera)=>[t('settings_remove_camera').replace('{camera}',camera),t('settings_remove_pending')];
 assert.deepEqual(names(tja,'カメラ 2（サブカメラ）'),['カメラ 2（サブカメラ）を削除する','保存すると削除します']);
 assert.equal(ui.ja.settings_remove_undo,'取り消す');
 assert.equal(ui.en.settings_remove_camera.replace('{camera}','Camera 2 (Sub)'),'Remove Camera 2 (Sub)');
});

test('a foreground read and a finished save clear the pending removals',async()=>{
 const {applyLoaded,finishAccept}=await import('../assets/app/settings-flow.js');
 const s={data:null,values:null,baseline:'',errors:{},removedCameraIDs:['cam2'],removedRows:{cam2:{}},notice:'',applyNotice:'x',scenes:[],carryErrors:false};
 applyLoaded(s,{data:{values:{cameras:[]},field_locations:{}},scenes:[],page:'cameras',background:false,t:k=>k,normalize:v=>v});
 assert.deepEqual([s.removedCameraIDs,s.removedRows],[[],{}]);
 const x={baseline:'saved',notice:'',removedCameraIDs:['cam2'],removedRows:{cam2:{}}};
 await finishAccept({action:'save',result:{apply:'restart'},values:{a:2},preview:{},updateOBS:false,t:k=>k,update:fn=>fn(x),dispatch:()=>{},load:async()=>{},closeDialog:()=>{},page:'cameras'});
 assert.deepEqual([x.removedCameraIDs,x.removedRows],[[],{}]);
});

// ---- the sources page ----
test('leaving the sources page with unsaved changes asks first and then goes on; the delete confirmations are red windows',()=>{
 const sources=read('sources.js');
 assert.match(sources,/async function close\(\)\{if\(draftChanged\(\)&&!await askConfirm\(confirmSpec\(t,\{title:t\('src_discard'\),lines:\[t\('confirm_discard_note'\)\],danger:true\}\)\)\)return false;leaveEditor\(\);queueMicrotask\(\(\)=>focusHeading\(root\)\);return true;\}/);
 // the page change is vetoed at once (the answer comes later), and made again after 「はい」
 assert.match(sources,/'keeper-before-page',e=>\{if\(e\.detail==='sources'\)return;if\(!draftChanged\(\)\)\{leaveEditor\(\);return;\}e\.preventDefault\(\);close\(\)\.then\(left=>\{if\(left\)window\.dispatchEvent\(new CustomEvent\('keeper-navigate',\{detail:e\.detail\}\)\);\}\);\}\);/);
 assert.match(sources,/const askDelete=\(item,opener\)=>\{const \{title,lines\}=deleteParts\(item,store\.get\(\)\.list,t\);return askConfirm\(confirmSpec\(t,\{title,lines,danger:true\}\),opener\);\};/);
 assert.match(sources,/async function remove\(src,opener\)\{if\(!await askDelete\(src,opener\)\)return;/);
 assert.match(sources,/if\(await askDelete\(\{\.\.\.file,used_by:e\.used_by\},opener\)\)/);
 // every delete button is red
 for(const m of sources.matchAll(/<button class="([^"]*)" (?:aria-label=[^@]*)?@click=\$\{e=>(?:removeFile\(f,e\.currentTarget\)|remove\(src,e\.currentTarget\))\}>/g))assert.equal(m[1],'btn stop');
 assert.equal([...sources.matchAll(/class="btn stop" (?:aria-label=[^@]*)?@click=\$\{e=>(?:removeFile|remove)\(/g)].length,3);
});

test('the shell navigates when it is told to (the sources page asks it again after 「はい」)',()=>{
 assert.match(read('shell.js'),/window\.addEventListener\('keeper-navigate',e=>navigate\(e\.detail\)\)/);
});

// ---- the colours of the buttons (spec v0.37) ----
test('the red of the ending buttons is the one of .btn.stop, and no rule gives .stop a margin of its own',()=>{
 const css=readFileSync(new URL('../assets/style.css',import.meta.url),'utf8');
 assert.match(css,/\.btn\.stop \{ border-color: var\(--down\); color: var\(--down\); \}/);
 assert.doesNotMatch(css,/\.stop\{margin-top:40px/);assert.doesNotMatch(css,/\.row \.stop/);
 assert.match(css,/\.confirm-yes\.stop\{background:var\(--down\)/);
 assert.match(css,/\.confirm-dialog\{[^}]*border-radius:12px/);
 assert.match(css,/\.source-preview iframe \{[^}]*color-scheme:light;/);
});

// ---- the texts of the login and of MediaMTX in the table the page reads ----
test('the table has the words of the login failure and of every MediaMTX state, and the status field never shows a raw value',()=>{
 assert.equal(ui.ja.mediamtx_down,'止まっている');assert.equal(ui.en.mediamtx_down,'Stopped');
 assert.equal(ui.ja.mediamtx_failed,'起動に失敗した');assert.equal(ui.en.mediamtx_failed,'Failed to start');
 for(const lang of ['ja','en'])for(const state of ['running','starting','down'])assert.ok(ui[lang]['mediamtx_'+state],lang+' '+state);
 const main=read('main.js');
 assert.match(main,/<dd id="mediamtx">\$\{t\('mediamtx_'\+\(data\.mediamtx\|\|'stopped'\)\)\|\|'—'\}<\/dd>/);
 const messages=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).messages;
 assert.equal(messages('ja')['api.wrong_password'],'パスワードが違います。');assert.equal(messages('en')['api.wrong_password'],'Wrong password.');
 assert.equal(messages('ja')['api.unauthorized'],'ログインが必要です。');
 assert.equal(messages('ja')['api.too_many_attempts'],'パスワードの誤りが続きました。しばらく待ってから、もう一度試してください。');
});
