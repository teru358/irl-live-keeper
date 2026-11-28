// S2-93: the page in the URL and Back, the phone menu as a window, the focus after a move, the size of what is pressed, the switch that cannot be pressed,
// and the confirmation windows whose title and lines said the same words.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {pages,pageFromHash,hashOf,createRouter,visibleHeading,focusHeading,createMenu,menuLabel} from '../assets/app/navigation.js';
import {confirmationSpec} from '../assets/app/settings-state.js';
import {deleteParts,obsSwitchState} from '../assets/app/sources-state.js';

const dir=new URL('../assets/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
const ui={ja:locale('ja'),en:locale('en')};
const tja=key=>ui.ja[key],ten=key=>ui.en[key];

// ---- a window with a history, like the browser's ----
function fakeWindow(hash=''){
 const listeners={},entries=[hash];let index=0;
 const fire=type=>(listeners[type]||[]).forEach(f=>f());
 return {
  entries,
  get index(){return index;},
  location:{get hash(){return entries[index];}},
  history:{
   pushState(_state,_title,url){entries.splice(index+1);entries.push(url);index++;},
   replaceState(_state,_title,url){entries[index]=url;},
  },
  addEventListener(type,f){(listeners[type]??=[]).push(f);},
  back(){index--;fire('popstate');fire('hashchange');},
  forward(){index++;fire('popstate');fire('hashchange');},
  // the person types a hash in the address bar: a new entry, and only hashchange
  type(url){entries.splice(index+1);entries.push(url);index++;fire('hashchange');},
 };
}
// The page the way the shell has it. The sources page keeps a draft: leaving it with changes is refused, and asks (answer('yes') goes on, as keeper-navigate does).
function app(hash='',{dirty=false}={}){
 const win=fakeWindow(hash),shown=[],asked=[];
 const state={page:pageFromHash(hash),draft:dirty};
 const router=createRouter({win,current:()=>state.page,before:page=>{
  if(page==='sources'||state.page!=='sources'||!state.draft)return true;
  asked.push(page);return false;
 },show:page=>{state.page=page;shown.push(page);}});
 return {win,state,router,shown,asked,answer(yes){const page=asked.pop();if(yes){state.draft=false;router.go(page);}}};
}

test('the page comes from the hash: an empty or unknown hash is the status page',()=>{
 assert.deepEqual(pages,['status','cameras','sources','settings']);
 for(const page of pages)assert.equal(pageFromHash('#'+page),page);
 for(const hash of ['','#','#nothing','#Cameras','#cameras/x',undefined,null])assert.equal(pageFromHash(hash),'status',String(hash));
 assert.equal(hashOf('cameras'),'#cameras');
});

test('opening the page with a hash starts on that page (a reload, a link, a change of language keeps the page)',()=>{
 for(const page of ['cameras','sources','settings']){
  const a=app('#'+page);
  assert.equal(a.router.initial(),page);assert.equal(a.state.page,page);
 }
 assert.equal(app('').router.initial(),'status');
 assert.equal(app('#oops').router.initial(),'status');
});

test('a move by the person writes the page into the URL and a history entry; the same page writes nothing more',()=>{
 const a=app('');
 assert.equal(a.router.go('cameras'),true);
 assert.deepEqual([a.state.page,a.win.entries,a.win.index],['cameras',['','#cameras'],1]);
 a.router.go('cameras');
 assert.deepEqual(a.win.entries,['','#cameras']);
 a.router.go('settings');a.router.go('sources');
 assert.deepEqual(a.win.entries,['','#cameras','#settings','#sources']);
 // the status page at the address with no hash is not written again
 const b=app('');
 b.router.go('status');
 assert.deepEqual(b.win.entries,['']);
 assert.equal(b.router.go('nothing'),false);
 assert.deepEqual([b.state.page,b.shown],['status',['status']]);
});

test('Back and forward walk the pages, and write nothing',()=>{
 const a=app('');
 a.router.go('cameras');a.router.go('settings');
 a.win.back();
 assert.deepEqual([a.state.page,a.win.entries],['cameras',['','#cameras','#settings']]);
 a.win.back();
 assert.deepEqual([a.state.page,a.win.index],['status',0]);
 a.win.forward();
 assert.equal(a.state.page,'cameras');
 // popstate and hashchange both arrive, the page is shown once
 assert.deepEqual(a.shown,['cameras','settings','cameras','status','cameras']);
 // a moved-to page after Back drops the forward entries, like the browser
 a.router.go('sources');
 assert.deepEqual(a.win.entries,['','#cameras','#sources']);
});

test('a hash typed in the address bar changes the page',()=>{
 const a=app('');
 a.win.type('#settings');
 assert.equal(a.state.page,'settings');
 a.win.type('#bogus');
 assert.equal(a.state.page,'status');
});

test('Back from a page that keeps a draft asks first: 「いいえ」 stays on the page and the URL, 「はい」 goes on',()=>{
 const a=app('',{dirty:true});
 a.router.go('cameras');
 a.router.go('sources');
 assert.deepEqual([a.state.page,a.win.entries],['sources',['','#cameras','#sources']]);
 a.win.back();
 // refused: the page stays, the question is asked, and the URL is the page that stays
 assert.deepEqual([a.state.page,a.asked,a.win.location.hash],['sources',['cameras'],'#sources']);
 a.answer(false);
 assert.deepEqual([a.state.page,a.win.location.hash],['sources','#sources']);
 // the refused entry now says the page that stayed; Back again goes on to the page before it, and this time 「はい」
 a.win.back();
 assert.deepEqual([a.state.page,a.asked],['sources',['status']]);
 a.answer(true);
 assert.deepEqual([a.state.page,a.win.location.hash],['status','#status']);
 assert.equal(a.state.draft,false);
});

test('leaving a page that keeps a draft from the menu is refused and writes nothing, until 「はい」',()=>{
 const a=app('#sources',{dirty:true});
 assert.equal(a.router.go('settings'),false);
 assert.deepEqual([a.state.page,a.win.entries,a.asked],['sources',['#sources'],['settings']]);
 a.answer(true);
 assert.deepEqual([a.state.page,a.win.entries],['settings',['#sources','#settings']]);
 // the same page needs no question
 const b=app('#sources',{dirty:true});
 assert.equal(b.router.go('sources'),true);
 assert.deepEqual(b.asked,[]);
});

// ---- the focus after a move ----
function fakeHeading(visible,name=''){
 return {name,visible,attributes:{},focused:0,getClientRects(){return this.visible?[{}]:[];},setAttribute(k,v){this.attributes[k]=v;},focus(){this.focused++;}};
}
const fakePanel=(...headings)=>({querySelectorAll:selector=>selector==='h2'?headings:[]});

test('after a move the heading of the page takes the focus: the first one that is on the screen, with tabindex -1',()=>{
 const hidden=fakeHeading(false,'links'),shown=fakeHeading(true,'scenes'),later=fakeHeading(true,'details');
 const panel=fakePanel(hidden,shown,later);
 assert.equal(visibleHeading(panel),shown);
 assert.equal(focusHeading(panel),true);
 assert.deepEqual([hidden.focused,shown.focused,later.focused,shown.attributes.tabindex,hidden.attributes.tabindex],[0,1,0,'-1',undefined]);
 assert.equal(focusHeading(fakePanel(fakeHeading(false))),false);
 assert.equal(focusHeading(null),false);
});

// ---- the phone menu as a window ----
function fakeMenu(){
 const listeners={},doc={activeElement:null,addEventListener(type,f){listeners[type]=f;},removeEventListener(type){delete listeners[type];}};
 const button=name=>({name,hidden:false,disabled:false,attrs:{},focus(){doc.activeElement=this;}});
 const phone={...button('phone'),hidden:true},status=button('status'),cameras=button('cameras'),logout=button('logout');
 const items=[phone,status,cameras,logout];
 const sheet={opened:0,closed:0,show(){this.opened++;},close(){this.closed++;},querySelectorAll:()=>items,querySelector:selector=>selector==='[aria-current="page"]'?cameras:null};
 const menuButton=button('menu'),behind=[{inert:false},{inert:false}];let closeAsked=0;
 const menu=createMenu({sheet,button:menuButton,background:()=>behind,close:()=>{closeAsked++;},doc});
 const press=(key,shiftKey=false)=>{let prevented=false;listeners.keydown?.({key,shiftKey,preventDefault(){prevented=true;}});return prevented;};
 return {doc,sheet,menu,menuButton,behind,items:{phone,status,cameras,logout},press,listeners,get closeAsked(){return closeAsked;}};
}

test('the menu opens as a window: it is shown, the page behind cannot be reached, and the focus moves to the row of the page that is open',()=>{
 const m=fakeMenu();
 m.menu.sync(true);
 assert.deepEqual([m.sheet.opened,m.menu.open,m.behind.map(b=>b.inert),m.doc.activeElement],[1,true,[true,true],m.items.cameras]);
 m.menu.sync(true);
 assert.equal(m.sheet.opened,1);
});

test('Escape closes the menu (it asks its owner), and the focus goes back to the 「メニュー」 button when it closes',()=>{
 const m=fakeMenu();
 m.menu.sync(true);
 assert.equal(m.press('a'),false);
 assert.equal(m.press('Escape'),true);
 assert.equal(m.closeAsked,1);
 m.menu.sync(false);
 assert.deepEqual([m.sheet.closed,m.menu.open,m.behind.map(b=>b.inert),m.doc.activeElement,'keydown' in m.listeners],[1,false,[false,false],m.menuButton,false]);
 // closed already: Escape does nothing to it
 assert.equal(m.press('Escape'),false);
});

test('Tab from the last item goes to the 「メニュー」 button, not out of the window; the hidden row is skipped',()=>{
 const m=fakeMenu();
 m.menu.sync(true);
 m.items.logout.focus();
 assert.equal(m.press('Tab',true),false);
 assert.equal(m.press('Tab'),true);
 assert.equal(m.doc.activeElement,m.menuButton);
 m.items.cameras.focus();
 assert.equal(m.press('Tab'),false);
 // with no row of the page, the first row that is shown takes the focus (the phone row is hidden)
 const n=fakeMenu();n.sheet.querySelector=()=>null;n.menu.sync(true);
 assert.equal(n.doc.activeElement,n.items.status);
});

test('while the menu is open the button says 「閉じる」, then 「メニュー」 again',()=>{
 assert.equal(menuLabel(tja,false),'メニュー');assert.equal(menuLabel(tja,true),'閉じる');
 assert.equal(menuLabel(ten,false),'Menu');assert.equal(menuLabel(ten,true),'Close');
});

test('the shell draws the menu as a dialog with a name, marks the open page in both menus, and starts from the hash',()=>{
 const shell=read('app/shell.js'),main=read('app/main.js');
 assert.match(shell,/<dialog id="sheet" class="sheet" aria-label=\$\{t\("menu"\)\}>/);
 assert.ok(!/<div id="sheet"/.test(shell)&&!/role="dialog"/.test(shell.replace(/<dialog[^>]*>/g,'')));
 assert.match(shell,/\$\{menuLabel\(t,s\.menu\)\}<\/button>/);
 // every row of both menus has the mark of the page that is open (the CSS takes it from aria-current)
 assert.equal((shell.match(/aria-current=\$\{s\.page==="(status|cameras|sources|settings)"\?"page":nothing\}/g)||[]).length,8);
 // Escape, focus and the page behind are the menu's; the state follows the window when it closes by itself (and when the screen gets wide)
 assert.match(shell,/createMenu\(\{sheet:/);
 assert.match(shell,/phoneMenu\(\)\.sync\(s\.menu&&!s\.desktop\)/);
 assert.match(shell,/addEventListener\('close',/);
 assert.match(shell,/desktop\.addEventListener\('change',\(\)=>change\(s=>\{s\.desktop=desktop\.matches;if\(s\.desktop\)s\.menu=false;\}\)\)/);
 // the page comes from the hash at the start, and the pages that load by keeper-page are told after they are mounted
 assert.match(shell,/const initialPage=pageFromHash\(location\.hash\);/);
 assert.match(shell,/createStore\(\{menu:false,drafts:\{\},page:initialPage,/);
 assert.match(main,/const shell=mountShell\(/);
 assert.ok(main.indexOf('shell.start()')>main.indexOf("window.addEventListener('keeper-page'"));
 assert.match(shell,/function start\(\)\{if\(!boot\.login&&store\.get\(\)\.page!=='status'\)window\.dispatchEvent\(new CustomEvent\('keeper-page',\{detail:store\.get\(\)\.page\}\)\);\}/);
 // the focus after a move comes after the draw: the menu gives the focus back first, then the heading takes it
 assert.ok(shell.indexOf('phoneMenu().sync(')<shell.indexOf('focusHeading(document'));
 assert.match(shell,/focusPage=page/);
});

test('the heading of the sources page takes the focus after 「新しく作る」, 「編集する」 and 「一覧へ戻る」',()=>{
 const sources=read('app/sources.js');
 assert.match(sources,/import \{focusHeading\} from '\.\/navigation\.js'/);
 const after=name=>sources.slice(sources.indexOf(name),sources.indexOf(name)+700);
 assert.match(after('async function open(src)'),/queueMicrotask\(\(\)=>focusHeading\(root\)\)/);
 assert.match(after('async function close()'),/leaveEditor\(\);queueMicrotask\(\(\)=>focusHeading\(root\)\);return true;/);
});

// ---- the size of what is pressed (the numbers are measured in the browser; here the rules are) ----
const css=read('style.css');
const flat=css.replace(/\s+/g,'');
// the bodies of every rule that has exactly this selector (a selector list is matched as written)
const rule=selector=>{
 const escaped=selector.replace(/\s+/g,'').replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 const bodies=[...flat.matchAll(new RegExp('(?:^|[}/])'+escaped+'\\{([^}]*)\\}','g'))].map(m=>m[1]);
 assert.ok(bodies.length,selector);
 return bodies.join(';');
};

test('the open page is marked in the phone menu, whose rows are buttons',()=>{
 assert.match(rule('.sheet-nav button[aria-current="page"]'),/font-weight:700;box-shadow:inset4px00var\(--ink\)/);
 assert.ok(!css.includes('.sheet-nav a[aria-current'));
});
test('the open menu is the dialog that is open; the closed one is not drawn, and the wide screen has no menu',()=>{
 assert.match(rule('.sheet[open]'),/^display:flex/);
 assert.ok(!/\.sheet\.open/.test(css));
 for(const part of ['margin:0','max-width:none','max-height:none','border:0','top:var(--top-h)'])assert.ok(rule('.sheet').includes(part),part);
 assert.match(flat,/@media\(min-width:900px\)\{[^@]*\.menu-btn,\.sheet,\.sheet\[open\]\{display:none;?\}/);
});
test('details headings, switches, segmented buttons, the menu button and the eye are 44px or more',()=>{
 assert.match(rule('main summary'),/min-height:44px/);
 assert.match(rule('.seg button'),/min-height:44px/);
 assert.match(rule('.seg button'),/font-size:14px/);
 assert.match(rule('.source-test .seg button'),/font-size:14px/);
 assert.match(rule('input[role=switch]'),/width:48px;height:44px;min-height:44px/);
 assert.match(rule('.menu-btn'),/min-height:44px/);
 assert.match(rule('.password-eye'),/width:44px/);
 assert.match(rule('.password-eye'),/min-height:44px/);
 assert.match(rule('.source-obs label, .source-free label'),/min-height:44px/);
 assert.ok(!/font-size:12px;padding:6px8px/.test(flat));
});
test('the height of the header is one number, used by what sticks under it and by what is scrolled to',()=>{
 assert.match(flat,/--top-h:65px/);
 assert.match(rule('.top'),/min-height:var\(--top-h\)/);
 assert.match(rule('.summary'),/top:var\(--top-h\)/);
 assert.match(rule('.sheet'),/top:var\(--top-h\)/);
 assert.ok(!/top:61px/.test(flat));
 assert.match(rule('.settings-card'),/scroll-margin-top:calc\(var\(--top-h\)\+12px\)/);
 assert.match(rule('main h2'),/scroll-margin-top:calc\(var\(--top-h\)\+12px\)/);
});
test('a switch that cannot be pressed is faint and says so with the cursor',()=>{
 assert.equal(rule('input[role=switch]:disabled'),'opacity:.5;cursor:not-allowed');
});

// ---- the switch 「OBS にも反映」 says why it cannot be pressed ----
test('the OBS switch: not pressable while streaming, or before a situation of the test is chosen, and each has its reason',()=>{
 assert.deepEqual(obsSwitchState({streaming:false,test:false,stream:'actual',line:'actual',standby:'actual',free:false,custom:''}),{disabled:true,reason:'src_obs_need_test'});
 assert.deepEqual(obsSwitchState({streaming:false,test:false,stream:'actual',line:'bad',standby:'actual',free:false,custom:''}),{disabled:false,reason:''});
 assert.deepEqual(obsSwitchState({streaming:true,test:false,stream:'actual',line:'bad',standby:'actual',free:false,custom:''}),{disabled:true,reason:'src_obs_streaming'});
 // already on: it can be turned off
 assert.deepEqual(obsSwitchState({streaming:false,test:true,stream:'actual',line:'actual',standby:'actual',free:false,custom:''}),{disabled:false,reason:''});
 assert.equal(ui.ja.src_obs_need_test,'テストの状況を選ぶと、OBS にも映せます。');
 assert.equal(ui.en.src_obs_need_test,'Choose a test situation to show it in OBS too.');
 for(const key of ['src_obs_need_test','src_obs_streaming','menu_close']){assert.ok(ui.ja[key],key);assert.ok(ui.en[key],key);}
 const sources=read('app/sources.js');
 assert.match(sources,/const state=obsSwitchState\(s\);/);
 assert.match(sources,/\?disabled=\$\{state\.disabled\} aria-describedby=\$\{state\.reason\?'source-obs-reason':nothing\}/);
 assert.match(sources,/state\.reason\?html`<p id="source-obs-reason" class="hint">\$\{t\(state\.reason\)\}<\/p>`:''/);
});

// ---- the title and the lines of a confirmation window do not say the same words twice ----
const words=text=>text.toLowerCase().replace(/[^a-z0-9’' ]/g,' ').split(/\s+/).filter(Boolean);
const stop=new Set(['the','a','an','of','to','will','be','is','are','do','not','by','that','were','this','in','on','it','you','will','can','be']);
const stem=w=>w.replace(/(ing|es|s|e|ed)$/,'');
const contentWords=text=>new Set(words(text).filter(w=>!stop.has(w)).map(stem));
// the longest run of characters that both texts have
function commonRun(a,b){
 let best=0;const row=new Array(b.length+1).fill(0);
 for(let i=1;i<=a.length;i++){for(let j=b.length;j>=1;j--){row[j]=a[i-1]===b[j-1]?row[j-1]+1:0;if(row[j]>best)best=row[j];}row[0]=0;}
 return best;
}
const noRepeat=(lang,{title,lines},label)=>{
 for(const line of lines){
  if(lang==='ja')assert.ok(commonRun(title,line)<6,`${label}: 「${title}」 and 「${line}」 say the same words`);
  else{const a=contentWords(title),shared=[...contentWords(line)].filter(w=>a.has(w));assert.ok(shared.length<2,`${label}: "${title}" and "${line}" share ${shared}`);}
 }
};
const saved={cameras:[{id:'cam1',name:''},{id:'cam2',name:'サブカメラ'}]};
// every operation of the settings windows, with and without a stream
const operations=[
 ['restart',{},{apply:'restart'}],
 ['save',{values:{}},{apply:'restart',fields:[{path:'admin.port'}]}],
 ['save',{values:{}},{apply:'immediate',fields:[{path:'log.keep_days'}]}],
 ['save',{values:{}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'removed'}]}],
 ['save',{values:{}},{apply:'restart',fields:[{path:'cameras[cam2]',kind:'added'},{path:'cameras[cam1].name'}]}],
 ['regenerate',{target:'admin'},{apply:'immediate'}],
 ['regenerate',{target:'read'},{apply:'restart',obs_url_changes:true}],
 ['regenerate',{target:'publish',camera:'cam1'},{apply:'restart'}],
];
test('the settings windows do not repeat the title in their lines (ja and en, with and without a stream)',()=>{
 for(const [lang,t] of [['ja',tja],['en',ten]])for(const [action,payload,preview] of operations)for(const streaming of [false,true]){
  const spec=confirmationSpec(action,payload,preview,streaming,t,saved);
  noRepeat(lang,spec,[lang,action,payload.target||'',preview.apply,streaming?'streaming':'idle'].join(' '));
 }
});
test('a restart window says what happens, not the question again; while streaming the second line starts with what the stream will feel',()=>{
 for(const [lang,t] of [['ja',tja],['en',ten]]){
  const idle=confirmationSpec('restart',{},{apply:'restart'},false,t,saved),live=confirmationSpec('restart',{},{apply:'restart'},true,t,saved);
  assert.equal(idle.lines.length,1);assert.equal(live.lines.length,2);
  assert.equal(idle.lines[0],t('settings_restart_only'));assert.equal(live.lines[1],t('settings_stream_warning'));
 }
 assert.equal(ui.ja.settings_restart_only,'設定は変わりません。');
 assert.equal(ui.en.settings_restart_only,'Settings do not change.');
 assert.ok(ui.ja.settings_stream_warning.startsWith('カメラからの受信が数秒切れ'));
 assert.ok(ui.en.settings_stream_warning.startsWith('Camera reception will pause'));
 assert.ok(!ui.ja.settings_stream_warning.includes('本体を再起動します'));
 assert.ok(!ui.en.settings_stream_warning.includes('The app will restart'));
});
test('the windows of 「すべて戻す」, 「捨てる」, deleting and 「LAN から開く」 do not repeat their title either',()=>{
 for(const [lang,t] of [['ja',tja],['en',ten]]){
  noRepeat(lang,{title:t('show_all_confirm'),lines:[t('confirm_show')]},lang+' show all');
  noRepeat(lang,{title:t('src_discard'),lines:[t('confirm_discard_note')]},lang+' discard');
  // the notice of the LAN is long and names the LAN once in the middle of what it says; only the Japanese words are held to the rule
  if(lang==='ja')noRepeat(lang,{title:t('confirm_lan'),lines:[t('settings_lan_notice')]},lang+' lan');
  const parts=deleteParts({name:'a',title:'Logo',used_by:['b']},[{name:'b',title:'Main'}],t);
  noRepeat(lang,parts,lang+' delete');
 }
 // the stream windows are the same kind of repeat (「配信を開始しますか？」 / 「OBS の配信を開始します。」) but are fixed by S2-91; they stay as they are
});
