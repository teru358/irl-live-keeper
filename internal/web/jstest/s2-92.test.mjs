// S2-92: the words (spec chapter 14, v0.37), the verbs of the buttons, the lines of the confirmation window, and the red window while streaming.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {showConfirm} from '../assets/app/confirm-dialog.js';
import {confirmationSpec,confirmationLines,confirmationDanger,settingText,specificSettings} from '../assets/app/settings-state.js';

const dir=new URL('../assets/app/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8'));
const ui={ja:locale('ja').ui,en:locale('en').ui};
const tja=key=>ui.ja[key],ten=key=>ui.en[key];
const saved={cameras:[{id:'cam1',name:''},{id:'cam2',name:'サブカメラ'}]};

// ---- the window while streaming: the operations that break the stream's reception are red and open on 「いいえ」 ----
const operations={
 restart:['restart',{},{apply:'restart'}],
 saveRestart:['save',{values:{}},{apply:'restart',fields:[{path:'admin.port'}]}],
 regenerateRead:['regenerate',{target:'read'},{apply:'restart',obs_url_changes:true}],
 regeneratePublish:['regenerate',{target:'publish',camera:'cam1'},{apply:'restart'}],
 saveImmediate:['save',{values:{}},{apply:'immediate',fields:[{path:'log.keep_days'}]}],
 regenerateAdmin:['regenerate',{target:'admin'},{apply:'immediate'}],
};
test('while streaming, a restart, a save that restarts, and a password that restarts are red and open on 「いいえ」',()=>{
 for(const name of ['restart','saveRestart','regenerateRead','regeneratePublish']){
  const [action,payload,preview]=operations[name];
  const spec=confirmationSpec(action,payload,preview,true,tja,saved);
  assert.deepEqual([spec.danger,spec.yesClass,spec.focus,spec.yesLabel,spec.noLabel],[true,'btn stop','no','はい','いいえ'],name);
  // the lines warn about the same thing (the reception drops)
  assert.ok(spec.lines.includes(ui.ja.settings_stream_warning),name);
 }
});
test('not streaming, the same windows are blue and open on 「はい」',()=>{
 for(const name of Object.keys(operations)){
  const [action,payload,preview]=operations[name];
  const spec=confirmationSpec(action,payload,preview,false,tja,saved);
  assert.deepEqual([spec.danger,spec.yesClass,spec.focus],[false,'btn primary','yes'],name);
 }
});
test('while streaming, an operation that does not restart the app stays blue (it cuts nothing)',()=>{
 for(const name of ['saveImmediate','regenerateAdmin']){
  const [action,payload,preview]=operations[name];
  const spec=confirmationSpec(action,payload,preview,true,tja,saved);
  assert.deepEqual([spec.danger,spec.yesClass,spec.focus],[false,'btn primary','yes'],name);
 }
});
test('a save that deletes a camera is red also when nothing is streaming',()=>{
 const preview={apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'}]};
 assert.equal(confirmationDanger(preview,'save',false),true);
 assert.equal(confirmationDanger({apply:'immediate',fields:[]},'save',false),false);
 assert.equal(confirmationDanger({apply:'immediate',fields:[]},'save',true),false);
});
test('the red window of a streaming restart takes the first focus on 「いいえ」',()=>{
 const focus=name=>({name,focused:0,focus(){this.focused++;}});
 for(const [streaming,focused] of [[true,'no'],[false,'yes']]){
  const yes=focus('yes'),no=focus('no'),listeners={};
  const dialog={showModal(){},querySelector:s=>s==='[data-confirm="yes"]'?yes:no,addEventListener(type,f){listeners[type]=f;},removeEventListener(){},contains:()=>false};
  showConfirm(dialog,confirmationSpec('restart',{},{apply:'restart'},streaming,tja,saved),null,{doc:{},defer:fn=>fn()});
  assert.deepEqual([yes.focused,no.focused],focused==='yes'?[1,0]:[0,1],String(streaming));
 }
});
test('settings.js hands the page streaming state to the decision (the window is drawn by confirmationSpec)',()=>{
 assert.match(read('settings.js'),/confirmationSpec\(s\.action,s\.payload,s\.preview,s\.streaming,t,/);
});

// ---- the words of the buttons: verbs ----
test('the buttons are verbs; adding a camera stays 「追加」; the answers of a window stay 「はい」「いいえ」',()=>{
 assert.deepEqual(Object.fromEntries(['settings_save','src_save','settings_remove','src_delete','src_edit','sender_copy','src_condition','settings_add','confirm_yes','confirm_no','settings_reload','settings_restart'].map(k=>[k,ui.ja[k]])),
  {settings_save:'保存する',src_save:'保存する',settings_remove:'削除する',src_delete:'削除する',src_edit:'編集する',sender_copy:'コピーする',src_condition:'条件を差し込む',settings_add:'追加',confirm_yes:'はい',confirm_no:'いいえ',settings_reload:'読み直す',settings_restart:'本体を再起動する'});
});
test('the words of the page: 本体, 確認, 捨てる, 切替, アドレス, OBS の読み取り',()=>{
 for(const key of ['settings_group_stream','setting_stream'])assert.equal(ui.ja[key],'OBS の読み取り');
 assert.equal(ui.ja.settings_restart_note,'＊ 保存すると本体が再起動します');
 for(const [key,text] of Object.entries(ui.ja)){
  assert.ok(!/破棄|確かめ|住所/.test(text),key+': '+text);
  assert.ok(!/(?<!配信)アプリ(?!名)/.test(text),key+': '+text);
  assert.ok(!/ホスト(?!名)/.test(text),key+': '+text);
 }
 assert.deepEqual(['setting_host','setting_publish_host','setting_obs_read_host','sender_address','sender_endpoint'].map(tja),['アドレス','送信用アドレス','OBS 用アドレス','送り先のアドレス','アドレス']);
 assert.deepEqual(['setting_publish_user','setting_read_user','sender_user'].map(tja),['送信用ユーザー名','読み取りユーザー名','ユーザー名']);
});
test('English calls the program 「the app」 and reads naturally (D-1 to D-20)',()=>{
 for(const [key,text] of Object.entries(ui.en))assert.ok(!/application|keeper|＊/.test(text),key+': '+text);
 const want={links:'Connection',sender_link:'Publish URL',camera_destination:'Publish URL',setting_admin:'Admin page',setting_low_sec:'Seconds before LOW',setting_down_sec:'Seconds before DOWN',
  setting_app:'App name',setting_binary:'MediaMTX location',setting_dir:'Log folder',loss:'Loss',editor_close:'Close',setting_option_auto:'Auto',theme_auto:'Auto',
  settings_restart:'Restart the app',settings_restart_note:'* Saving restarts the app',phone_create:'Create QR code',stream:'Stream',src_line:'Connection',settings_saved:'Saving…'};
 for(const [key,text] of Object.entries(want))assert.equal(ui.en[key],text,key);
 // the same section is named alike on every side
 assert.equal(ui.en.settings_group_stream,ui.en.setting_stream);
 assert.ok(!/ $/.test(ui.en.settings_lan_notice)&&!/ $/.test(ui.ja.settings_lan_notice));
});

// ---- the lines of the confirmation window ----
test('the line that says how the save is applied is a sentence that names the app',()=>{
 assert.equal(ui.ja.settings_restart_mode,'保存すると、本体が再起動します。');
 assert.equal(ui.ja.settings_immediate,'保存すると、すぐに反映します。');
 assert.equal(ui.en.settings_restart_mode,'Saving restarts the app.');
 assert.equal(ui.en.settings_immediate,'Saving applies the changes immediately.');
 assert.equal(confirmationLines('save',{},{apply:'restart',fields:[]},false,tja)[0],'保存すると、本体が再起動します。');
 assert.equal(confirmationLines('save',{},{apply:'immediate',fields:[]},false,tja)[0],'保存すると、すぐに反映します。');
});
test('a camera that is deleted or added reads 「カメラ 2（サブカメラ）を削除」, not 「カメラ 2（サブカメラ） — 削除」',()=>{
 const lines=confirmationLines('save',{values:{cameras:[saved.cameras[0],{id:'cam3',name:''}]}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'},{path:'cameras[cam3]',kind:'added'},{path:'cameras[cam1].name'}]},false,tja,saved);
 assert.ok(lines.includes('カメラ 2（サブカメラ）を削除'),lines.join('|'));
 assert.ok(lines.includes('カメラ 3を追加'),lines.join('|'));
 assert.ok(lines.includes('カメラ 1 — 名前'),lines.join('|'));
 assert.ok(!lines.some(l=>/ — (削除|追加)$/.test(l)));
 const en=confirmationLines('save',{values:{cameras:[]}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'}]},false,ten,{cameras:[{id:'cam2',name:'Sub'}]});
 assert.ok(en.includes('Remove Camera 2 (Sub)'),en.join('|'));
});
test('the comments of config.yaml are lost only when a camera is saved, and that is said in the window (not on the page)',()=>{
 const withCamera=confirmationLines('save',{},{apply:'restart',fields:[{path:'cameras[cam1].name'}]},false,tja,saved);
 assert.ok(withCamera.includes(ui.ja.settings_camera_comments));
 assert.ok(!confirmationLines('save',{},{apply:'immediate',fields:[{path:'log.keep_days'}]},false,tja,saved).includes(ui.ja.settings_camera_comments));
 assert.ok(!confirmationLines('restart',{},{apply:'restart'},false,tja,saved).includes(ui.ja.settings_camera_comments));
 assert.ok(!read('settings.js').includes("t('settings_camera_comments')"));
});

// ---- the labels that depend on the place (C-9) ----
test('the port and the password of OBS and of the admin page have their own labels and helps; the switch has a label of its own',()=>{
 assert.deepEqual(specificSettings,['obs.port','obs.password','admin.port','admin.password','on_stream_start.enabled']);
 const label=(path,key)=>settingText(tja,path,key),help=(path,key)=>settingText(tja,path,key,'_help');
 assert.equal(label('obs.port','port'),'OBS の WebSocket のポート');
 assert.equal(label('admin.port','port'),'管理ページを開くポート');
 assert.equal(label('obs.password','password'),'OBS の WebSocket のパスワード');
 assert.equal(label('admin.password','password'),'管理ページのパスワード');
 assert.equal(label('on_stream_start.enabled','enabled'),'配信開始時にシーンを切り替える');
 assert.notEqual(help('obs.port','port'),help('admin.port','port'));
 assert.notEqual(help('obs.password','password'),help('admin.password','password'));
 // other paths use the last part, as before
 assert.equal(settingText(tja,'cameras[cam1].name','name'),'名前');
 assert.equal(settingText(ten,'obs.port','port'),'OBS WebSocket port');
 for(const lang of ['ja','en'])for(const key of ['setting_port','setting_password','setting_enabled','setting_port_help','setting_password_help','setting_enabled_help'])assert.ok(!(key in ui[lang]),lang+' '+key);
});
test('the note of the restart is written once on a page, and the mark is the text of the table',()=>{
 const settings=read('settings.js');
 assert.equal(settings.split("t('settings_restart_note')").length-1,1);
 // it is outside the loop over the groups (the cameras page shows two groups at once)
 assert.ok(settings.indexOf("t('settings_restart_note')")>settings.indexOf('Object.entries(groups).map(([g,keys])'));
 assert.ok(/<\/div>`\):''}\$\{s\.data\?html`<p class="settings-note">\$\{t\('settings_restart_note'\)\}<\/p>`:''\}/.test(settings));
 assert.ok(!settings.includes('＊'));
 assert.equal(ui.ja.settings_restart_mark,'＊');assert.equal(ui.en.settings_restart_mark,'*');
});

// ---- the places that say the same thing: OBS is not connected (C-13) ----
test('OBS not connected: the status says only that, and the scene list says what to do with a button to the OBS settings',()=>{
 const main=read('main.js'),settings=read('settings.js');
 assert.match(main,/o\.connected\?t\('obs_connected'\)\+separator\+t\(o\.studio_mode\?'studio_on':'studio_off'\):t\('obs_disconnected'\)/);
 assert.match(main,/openObsSettings/);
 assert.match(main,/keeper-settings-group/);
 assert.match(settings,/keeper-settings-group/);
 assert.equal(ui.ja.obs_disconnected,'つながっていない');
 assert.equal(ui.ja.scenes_disconnected,'OBS につながると、シーンがここに並びます。');
 assert.equal(ui.ja.scenes_open_settings,'OBS の設定を開く');
 assert.equal(ui.ja.settings_disconnected,'OBS につながると、シーンを選べます。名前を直接入力することもできます。');
});

// ---- the phone window (C-8): the words only ----
test('the phone window says what it makes: the address to open on the phone, 「QR を作る」, and 「閉じる」',()=>{
 const phone=read('phone.js');
 assert.match(phone,/t\('phone_address'\)/);assert.ok(!phone.includes("t('sender_address')"));
 assert.match(phone,/t\('phone_close'\)/);assert.ok(!phone.includes('settings_cancel'));
 assert.deepEqual(['phone_address','phone_create','phone_recreate','phone_close'].map(tja),['スマホから開くアドレス','QR を作る','QR を作り直す','閉じる']);
});

// ---- the name of a source (C-2) ----
test('the URL name of a source says which characters can be used and that it cannot be changed, under the field',()=>{
 const sources=read('sources.js'),field=read('source-field.js');
 assert.match(field,/const ids=\[key==='name'\?'source-name-help':''/);
 assert.match(field,/<small id="source-name-help" class="settings-help">\$\{t\('src_name_help'\)\}<\/small>/);
 // the helper is inside the label of the field, so it stays under the field
 assert.match(field,/<\/span>[^]*\$\{key==='name'\?html`<small id="source-name-help"[^]*?<\/small>`:''\}<\/label>/);
 assert.equal(ui.ja.src_title,'表示名');assert.equal(ui.ja.src_name,'URL 用の名前');
 assert.match(ui.ja.src_name_help,/英数字と - と _/);assert.match(ui.ja.src_name_help,/作ったあとは変えられません/);
 assert.match(ui.en.src_name_help,/cannot be changed/);
 // the test bar has a visible title
 assert.match(sources,/<span class="hint source-test-title">\$\{t\('src_test'\)\}<\/span>/);
});

// ---- the menu: a page with an unsaved draft is read with that (C-14) ----
test('the menu button of a page with a draft is read 「カメラ（保存していない変更あり）」',()=>{
 const shell=read('shell.js');
 assert.equal((shell.match(/aria-label=\$\{s\.drafts\.cameras\?t\("settings_unsaved_aria"\)/g)||[]).length,2);
 assert.equal((shell.match(/aria-label=\$\{s\.drafts\.settings\?t\("settings_unsaved_aria"\)/g)||[]).length,2);
 assert.equal(ui.ja.settings_unsaved_aria.replace('{page}','カメラ'),'カメラ（保存していない変更あり）');
});

// ---- the tables ----
test('both tables have the same keys',()=>{
 for(const section of ['ui','messages']){
  const ja=Object.keys(locale('ja')[section]).sort(),en=Object.keys(locale('en')[section]).sort();
  assert.deepEqual(ja,en,section);
 }
});
test('the errors that were English in the Japanese table are Japanese, and each says what to do (C-12)',()=>{
 const ja=locale('ja').messages,en=locale('en').messages;
 for(const code of ['api.deadline','api.canceled'])assert.ok(!/context/.test(ja[code])&&!/context/.test(en[code]),code);
 assert.equal(ja['api.deadline'],'時間内に終わりませんでした。もう一度試してください。');
 assert.equal(en['api.deadline'],'Timed out. Try again.');
 assert.equal(en['api.unauthorized'],'Please sign in.');
 assert.equal(en['api.stopped'],'The app has stopped. Restart it.');
 assert.match(ja['api.disconnected'],/OBS を起動し/);
 assert.match(ui.ja.disconnected,/自動でつなぎ直します/);
 assert.match(ui.ja.failed,/もう一度試してください/);
});
