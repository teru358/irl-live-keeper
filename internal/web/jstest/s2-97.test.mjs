// S2-97: the fixes after the real-machine check of S2-95: the errors of the name and the display name beside their fields, Back after 「いいえ」, the bar after a reload,
// the names that are read aloud, the separator of a language, the wrapping of a name, the hint of the manual standby, and the title of the camera save window.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,readdirSync} from 'node:fs';
import {createRouter,pageFromHash} from '../assets/app/navigation.js';
import {barSticky,saveBar} from '../assets/app/save-bar.js';
import {sourceField,saveFailed,errorField,clearFieldError,fieldErrorID} from '../assets/app/source-field.js';
import {clearEditor,openEditor} from '../assets/app/sources-state.js';
import {confirmationTitle,confirmationSpec} from '../assets/app/settings-state.js';

const dir=new URL('../assets/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
const ui={ja:locale('ja'),en:locale('en')};
const tja=key=>ui.ja[key],ten=key=>ui.en[key];

// ---- a lit-like html that keeps the text of what is drawn ----
function fakeHTML(){
 const text=v=>Array.isArray(v)?v.map(text).join(''):v&&typeof v==='object'?v.parts.map((p,i)=>p+(i<v.values.length?text(v.values[i]):'')).join(''):typeof v==='function'?'<fn>':v===undefined||v===null||v===false?'':String(v);
 return {html:(parts,...values)=>({parts:[...parts],values}),text};
}
const deps=f=>({html:f.html,live:v=>v,nothing:''});
const draft=(over={})=>({draft:{name:'',title:'',body:'',obs_width:800,obs_height:200,...over},fresh:true,busy:false,fieldErrors:{}});

// ---- 1, 2: the error of the name is under the field, the field says so, and the preview has nothing to do with it ----
test('the server codes of the name and of the display name belong to their fields; every other error stays the error of the edit',()=>{
 for(const code of ['sources.name_empty','sources.name_chars','sources.name_long','sources.reserved_name','sources.duplicate'])assert.equal(errorField(code),'name',code);
 assert.equal(errorField('sources.title_long'),'title');
 for(const code of ['sources.invalid','sources.size_range','sources.body_too_large','template.unknown',undefined,'api.unreachable'])assert.equal(errorField(code),'',String(code));
 const s=draft();
 const e=Object.assign(Error('URL 用の名前に使えるのは、英数字と - と _ だけです。'),{code:'sources.name_chars'});
 assert.equal(saveFailed(s,e),'name');
 assert.deepEqual(s.fieldErrors,{name:e.message});assert.equal(s.editorError??'','');
 const other=Object.assign(Error('サイズ'),{code:'sources.size_range'});
 assert.equal(saveFailed(s,other),'');assert.equal(s.editorError,'サイズ');
 // a saved source has no name field: its name error is the error of the edit
 const saved={...draft(),fresh:false};
 assert.equal(saveFailed(saved,e),'');assert.equal(saved.editorError,e.message);
 const title=Object.assign(Error('表示名が長すぎます。'),{code:'sources.title_long'});
 assert.equal(saveFailed(saved,title),'title');assert.equal(saved.fieldErrors.title,title.message);
});

test('the name field: the error is right under the field, the field is aria-invalid and points to the error and to the note',()=>{
 const f=fakeHTML();
 const view=(s,key='name')=>f.text(sourceField(deps(f),{s,key,t:tja,onInput(){}}));
 const ok=view(draft());
 assert.ok(!ok.includes('aria-invalid=true'));
 assert.match(ok,/aria-describedby=source-name-help /);
 assert.match(ok,/<small id=source-name-error class="warn source-field-error" role="alert"><\/small>/);
 const s={...draft({name:'夕焼け'}),fieldErrors:{name:'URL 用の名前に使えるのは、英数字と - と _ だけです。'}};
 const bad=view(s);
 assert.ok(bad.includes('aria-invalid=true'));
 assert.match(bad,/aria-describedby=source-name-help source-name-error /);
 assert.match(bad,/<small id=source-name-error class="warn source-field-error" role="alert">URL 用の名前に使えるのは/);
 // order: the input, then the error, then the note (the error is the first thing under the field)
 assert.ok(bad.indexOf('<input')<bad.indexOf('<small id=source-name-error')&&bad.indexOf('<small id=source-name-error')<bad.indexOf('<small id="source-name-help"'));
 assert.ok(bad.endsWith('</label>'));
 // the display name has the same: an error and no note
 const title={...draft(),fieldErrors:{title:'表示名が長すぎます。'}};
 const t1=view(title,'title');
 assert.ok(t1.includes('aria-invalid=true')&&t1.includes('aria-describedby=source-title-error ')&&t1.includes('id=source-title-error'));
 assert.ok(!view(draft(),'title').includes('aria-invalid=true'));
 assert.equal(fieldErrorID('name'),'source-name-error');
 // a number has no place for an error
 assert.ok(!view(draft(),'obs_width').includes('source-obs_width-error'));
});

test('typing in a field takes back the error of that field only; a new edit starts with none',()=>{
 const s={fieldErrors:{name:'a',title:'b'}};
 clearFieldError(s,'name');
 assert.deepEqual(s.fieldErrors,{name:'',title:'b'});
 const e={fieldErrors:{name:'x'},draft:{},base:{},valueGroups:[]};
 clearEditor(e);assert.deepEqual(e.fieldErrors,{});
 const o={fieldErrors:{name:'x'},defaults:{obs_width:1},broken:{}};
 openEditor(o,null);assert.deepEqual(o.fieldErrors,{});
});

test('sources.js: a failed save puts the error beside its field and moves the focus there; the preview error is not the name error',()=>{
 const src=read('app/sources.js');
 assert.match(src,/catch\(e\)\{let key='';update\(x=>\{key=saveFailed\(x,e\);\}\);if\(key\)setTimeout\(\(\)=>root\.querySelector\('\[name="source-'\+key\+'"\]'\)\?\.focus\(\),0\);\}/);
 assert.match(src,/const field=\(s,key,type='text'\)=>sourceField\(/);
 assert.match(src,/clearFieldError\(x,k\)/);
 // the save starts by taking back the old field errors
 assert.match(src,/x\.busy=true;x\.editorError='';x\.fieldErrors=\{\};/);
 assert.match(read('style.css'),/\.warn:empty\s*\{\s*display:\s*none/);
});

// ---- 3: Back, 「いいえ」, Back ----
function fakeWindow(hash=''){
 const listeners={},entries=[hash];let index=0;
 const fire=type=>(listeners[type]||[]).forEach(f=>f());
 return {entries,get index(){return index;},location:{get hash(){return entries[index];}},
  history:{pushState(_s,_t,url){entries.splice(index+1);entries.push(url);index++;},replaceState(_s,_t,url){entries[index]=url;}},
  addEventListener(type,f){(listeners[type]??=[]).push(f);},
  back(){index--;fire('popstate');fire('hashchange');},forward(){index++;fire('popstate');fire('hashchange');}};
}
test('Back, 「いいえ」, Back: the question comes again every time (the page that stayed is pushed again)',()=>{
 const win=fakeWindow(''),asked=[];
 const state={page:'status',draft:false};
 const router=createRouter({win,current:()=>state.page,before:page=>{if(page==='sources'||state.page!=='sources'||!state.draft)return true;asked.push(page);return false;},show:page=>{state.page=page;}});
 router.go('cameras');router.go('sources');state.draft=true;
 for(let round=1;round<=3;round++){
  win.back();
  assert.deepEqual([state.page,asked.length,win.location.hash],['sources',round,'#sources'],'round '+round);
  assert.equal(pageFromHash(win.location.hash),'sources');
 }
 assert.deepEqual(win.entries,['','#cameras','#sources']);
 // 「はい」: the draft goes and the page moves on
 win.back();state.draft=false;router.go(asked.pop());
 assert.equal(state.page,'cameras');
});
test('Forward from a page that keeps a draft is asked the same way',()=>{
 const win=fakeWindow(''),asked=[];
 const state={page:'status',draft:false};
 const router=createRouter({win,current:()=>state.page,before:page=>{if(page==='sources'||state.page!=='sources'||!state.draft)return true;asked.push(page);return false;},show:page=>{state.page=page;}});
 router.go('sources');router.go('settings');win.back();
 assert.equal(state.page,'sources');state.draft=true;
 win.forward();
 assert.deepEqual([state.page,asked,win.location.hash],['sources',['settings'],'#sources']);
 win.back();
 assert.equal(asked.length,2);
});

// ---- 4: the bar after a reload ----
test('the notice of a reload does not keep the bar stuck; the notice of a save, a failure and a failed apply do',()=>{
 assert.equal(barSticky({dirty:false,notice:'読み直しました。',quietNotice:'読み直しました。'}),false);
 assert.equal(barSticky({dirty:true,notice:'読み直しました。',quietNotice:'読み直しました。'}),true);
 assert.equal(barSticky({dirty:false,notice:'保存を受け付けました。',quietNotice:'読み直しました。'}),true);
 assert.equal(barSticky({dirty:false,notice:'',quietNotice:'読み直しました。'}),false);
 assert.equal(barSticky({dirty:false,notice:'',applyNotice:'x',quietNotice:'読み直しました。'}),true);
 const f=fakeHTML();
 const out=f.text(saveBar(f.html,{page:'cameras',dirty:false,busy:false,notice:'読み直しました。',quietNotice:'読み直しました。',t:tja,onSave(){},onReload(){}}));
 assert.ok(!out.includes('sticky'));
 // the notice is still above the buttons
 assert.ok(out.indexOf('読み直しました。')<out.indexOf('>保存する<'));
 const settings=read('app/settings.js');
 assert.match(settings,/x\.notice=t\('settings_reloaded'\);x\.quietNotice=x\.notice;/);
 assert.match(settings,/quietNotice:s\.quietNotice/);
});

// ---- 5: the names that are read aloud ----
test('the link to the destination and the buttons of a source say whose they are',()=>{
 assert.equal(ui.ja.sender_link_of.replace('{camera}','カメラ 1（メインカメラ）'),'カメラ 1（メインカメラ）の送り先');
 assert.equal(ui.en.sender_link_of.replace('{camera}','Camera 1 (メインカメラ)'),'Publish URL of Camera 1 (メインカメラ)');
 assert.equal(ui.ja.src_delete_of.replace('{name}','夕焼け'),'夕焼けを削除する');
 assert.equal(ui.ja.src_edit_of.replace('{name}','夕焼け'),'夕焼けを編集する');
 assert.equal(ui.en.src_delete_of.replace('{name}','Sunset'),'Delete Sunset');
 assert.equal(ui.en.src_edit_of.replace('{name}','Sunset'),'Edit Sunset');
 // the name read aloud starts with the words that are seen
 assert.ok(ui.en.src_delete_of.startsWith(ui.en.src_delete)&&ui.en.src_edit_of.startsWith(ui.en.src_edit)&&ui.en.sender_link_of.startsWith(ui.en.sender_link));
 assert.ok(ui.ja.sender_link_of.includes(ui.ja.sender_link)&&ui.ja.src_delete_of.endsWith(ui.ja.src_delete)&&ui.ja.src_edit_of.endsWith(ui.ja.src_edit));
 const main=read('app/main.js');
 assert.ok(main.includes("aria-label=${t('sender_link_of').replace('{camera}',()=>cameraHeading(c,t))}>${t('sender_link')}"));
 const sources=read('app/sources.js');
 assert.ok(sources.includes("aria-label=${t('src_edit_of').replace('{name}',()=>src.title||src.name)} @click=${()=>open(src)}>${t('src_edit')}"));
 assert.ok(sources.includes("aria-label=${t('src_delete_of').replace('{name}',()=>src.title||src.name)} @click=${e=>remove(src,e.currentTarget)}>${t('src_delete')}"));
});

// ---- 6: the separator of a language, the name that is not broken in the middle of a word ----
test('no Japanese comma is written into the code that draws a page; the names are joined by the separator of the language',()=>{
 const files=readdirSync(new URL('app/',dir)).filter(n=>n.endsWith('.js')).concat(['source.js']);
 for(const name of files){
  const text=read(name==='source.js'?name:'app/'+name).split('\n').filter(line=>!/^\s*\/\//.test(line)).map(line=>line.replace(/\s\/\/.*$/,'')).join('\n');
  const hits=text.split('\n').filter(line=>/[、，]/.test(line)).filter(line=>!line.includes("lang==='ja'?'、'"));
  assert.deepEqual(hits,[],name);
 }
 const main=read('app/main.js');
 assert.ok(main.includes("const separator=lang==='ja'?'、':', ';"));
 assert.ok(main.includes("scene.fallback_of.join(separator)"));
});
test('the name of a camera in the status is not broken in the middle of a word, and does not run out of its box',()=>{
 const css=read('style.css').replace(/\/\*[^]*?\*\//g,'');
 const rule=css.match(/\.cam-name\s*\{([^}]*)\}/)[1];
 assert.match(rule,/word-break:\s*keep-all/);assert.match(rule,/overflow-wrap:\s*anywhere/);
});

// ---- 7: the manual standby hint, 8: the title of the camera save window ----
test('the hint of the manual standby does not name a tab (the wide page has none)',()=>{
 assert.equal(ui.ja.manual_hold_hint,'シーンは変えずに、自動で戻らないようにします。戻るときは、シーン一覧の「〜に戻る」を押します。');
 assert.equal(ui.en.manual_hold_hint,'Keep this scene and stop automatic return. To go back, press "Return to …" in the scene list.');
 assert.ok(!/タブ|tab/i.test(ui.ja.manual_hold_hint+ui.en.manual_hold_hint));
});
test('the save window of the camera page says 「カメラの設定」; the other windows keep their titles',()=>{
 assert.equal(confirmationTitle('save',{},tja,'cameras'),'カメラの設定を保存しますか？');
 assert.equal(confirmationTitle('save',{},ten,'cameras'),'Save the camera settings?');
 assert.equal(confirmationTitle('save',{},tja,'settings'),'設定を保存しますか？');
 assert.equal(confirmationTitle('save',{},tja),'設定を保存しますか？');
 assert.equal(confirmationTitle('restart',{},tja,'cameras'),tja('confirm_restart'));
 const spec=confirmationSpec('save',{},{fields:[]},false,tja,{},null,[],'cameras');
 assert.equal(spec.title,'カメラの設定を保存しますか？');
 assert.match(read('app/settings.js'),/\.map\(\(\)=>t\('settings_local'\)\):\[\],page\):confirmSpec/);
});

test('both tables have the same keys',()=>{
 assert.deepEqual(Object.keys(ui.ja).sort(),Object.keys(ui.en).sort());
 for(const key of ['sender_link_of','src_edit_of','src_delete_of','confirm_save_cameras'])assert.ok(ui.ja[key]&&ui.en[key],key);
});
