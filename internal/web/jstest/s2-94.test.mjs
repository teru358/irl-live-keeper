// S2-94: the colours (the ratio is computed from style.css), the order of the headings, the save row and its sticky bar, the errors that are shown
// near what was pressed, the destination link, the preview and the editor, the shapes of the fields, and the small words of the windows.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {barSticky,saveBar,restartZone} from '../assets/app/save-bar.js';
import {confirmationSpec,confirmationLines,numberBounds} from '../assets/app/settings-state.js';
import {streamConfirm} from '../assets/app/stream-control.js';
import {previewRatio} from '../assets/app/sources-state.js';

const dir=new URL('../assets/',import.meta.url);
const read=name=>readFileSync(new URL(name,dir),'utf8');
const locale=lang=>JSON.parse(readFileSync(new URL('../../i18n/locales/'+lang+'.json',import.meta.url),'utf8')).ui;
const ui={ja:locale('ja'),en:locale('en')};
const tja=key=>ui.ja[key],ten=key=>ui.en[key];
const css=read('style.css');
const flat=css.replace(/\s+/g,'');

// ---- a lit-like html that keeps the text and the order of what is drawn ----
function fakeHTML(){
 const handlers=[];
 const text=v=>Array.isArray(v)?v.map(text).join(''):v&&typeof v==='object'?v.parts.map((p,i)=>p+(i<v.values.length?text(v.values[i]):'')).join(''):typeof v==='function'?'<fn#'+(handlers.push(v)-1)+'>':v===undefined||v===null?'':String(v);
 return {handlers,html:(parts,...values)=>({parts:[...parts],values}),text};
}

// ---- the colours: the ratio is computed from the variables of style.css ----
const rules=[...css.replace(/\/\*[^]*?\*\//g,'').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(m=>({selector:m[1].replace(/\s+/g,' ').trim(),body:m[2]}));
const variables=selectors=>{
 const out={};
 for(const r of rules)if(selectors.includes(r.selector))for(const m of r.body.matchAll(/(--[a-z-]+)\s*:\s*(#[0-9a-fA-F]{6})\b/g))out[m[1]]=m[2];
 return out;
};
const light=variables([':root']);
const dark=variables([':root[data-theme="dark"]',':root[data-theme=dark]']);
const autoDark=variables([':root:not([data-theme="light"])',':root:not([data-theme=light])']);
const lum=hex=>{const [r,g,b]=[1,3,5].map(i=>parseInt(hex.slice(i,i+2),16)/255).map(c=>c<=0.03928?c/12.92:((c+0.055)/1.055)**2.4);return 0.2126*r+0.7152*g+0.0722*b;};
const ratio=(a,b)=>{const [x,y]=[lum(a),lum(b)].sort((p,q)=>q-p);return (x+0.05)/(y+0.05);};

test('the two dark rules (the automatic one and the chosen one) have the same colours',()=>{
 for(const [name,value] of Object.entries(dark))assert.equal(autoDark[name],value,name);
 assert.ok(Object.keys(dark).length>=15);
});

// the text colours are read on the ground and on the surface of the theme
const textColours=['--ink','--muted','--ok','--low','--down','--act','--k-cam','--k-none','--k-standby','--source-value'];
for(const [theme,vars] of [['light',light],['dark',dark]]){
 test(`${theme}: every text colour is 4.5 or more on the ground and on the surface`,()=>{
  for(const name of textColours){
   for(const ground of ['--bg','--surface']){
    const r=ratio(vars[name],vars[ground]);
    assert.ok(r>=4.5,`${theme} ${name} on ${ground}: ${r.toFixed(2)}`);
   }
  }
 });
 test(`${theme}: the text of the buttons that are filled, of the red 「はい」 and of the tally is 4.5 or more`,()=>{
  assert.ok(ratio(vars['--act-ink'],vars['--act'])>=4.5,'act');
  assert.ok(ratio(vars['--down-ink']||dark['--down-ink']||'#FFFFFF',vars['--down'])>=4.5,'down');
  assert.ok(ratio(vars['--tally-ink'],vars['--tally'])>=4.5,'tally '+ratio(vars['--tally-ink'],vars['--tally']).toFixed(2));
 });
 test(`${theme}: the line of a field can be seen (3 or more) on the ground and on the surface`,()=>{
  for(const ground of ['--bg','--surface'])assert.ok(ratio(vars['--field-line'],vars[ground])>=3,theme+' field line on '+ground+' '+ratio(vars['--field-line'],vars[ground]).toFixed(2));
 });
}
test('the LOW colour of the light theme keeps the amber (the hue) of the colour that the spec allowed',()=>{
 const hue=hex=>{const [r,g,b]=[1,3,5].map(i=>parseInt(hex.slice(i,i+2),16)/255);const max=Math.max(r,g,b),min=Math.min(r,g,b),d=max-min;return d?(max===r?((g-b)/d)%6:max===g?(b-r)/d+2:(r-g)/d+4)*60:0;};
 assert.ok(Math.abs(hue(light['--low'])-hue('#A86A00'))<3,light['--low']);
 assert.notEqual(light['--low'],'#A86A00');
});
test('a number or a reason that is text is not thinned by opacity',()=>{
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.ok(!/opacity/.test(body('.tick')),'the tick line is thinned by its colour');
 assert.ok(!/opacity/.test(body('.tick span')));
 assert.ok(!/opacity/.test(body('.source-file.unusable')));
 assert.ok(!/opacity/.test(body('.source-file.unusable > .source-file-reason')));
 assert.match(body('.tick'),/color-mix\(in srgb, var\(--ink\) 55%, transparent\)/);
 // the tally takes the colour of its text from the theme, and the dot follows it
 assert.match(body('.tally'),/color: var\(--tally-ink\)/);
 assert.match(body('.tally::before'),/background: currentColor/);
 // the line of a field is the visible one
 for(const selector of ['.settings-field input:not([type=checkbox]),.settings-field select','#login input','.sender-field input'])assert.ok(rules.some(r=>r.selector.replace(/\s/g,'')===selector.replace(/\s/g,'')&&r.body.includes('var(--field-line)')),selector);
 assert.match(body('#sources-panel .cm-editor'),/var\(--field-line\)/);
});

// ---- the order of the headings ----
test('the title of a page is the largest and heaviest heading; the parts are 17px; the camera card is an h4; a window title is larger than its text',()=>{
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('h2'),/font-size: 20px/);assert.match(body('h2'),/font-weight: 700/);assert.match(body('h2'),/color: var\(--ink\)/);
 assert.match(body('main h3'),/font-size: 17px/);
 assert.match(body('.settings-card h4'),/font-size: 16px/);
 assert.match(body('.confirm-dialog h2'),/font-size:20px/);
 assert.match(body('body'),/font-size: 16px/);
 // the quiet small labels of the status page stay as they were
 assert.match(body('#status h2'),/font-size: 15px/);assert.match(body('#status h2'),/color: var\(--muted\)/);
 const settings=read('app/settings.js');
 assert.match(settings,/<section class="settings-card" id=\$\{'camera-'\+v\.id\}><h4>/);
 assert.match(settings,/<section class="settings-card removing" id=\$\{'camera-'\+row\.id\}><h4>/);
 assert.ok(!/class="settings-card"[^`]*<h3>/.test(settings.replace(/<fieldset[^]*?<\/fieldset>/g,'')));
});

// ---- the save row and the sticky bar ----
const bar=(f,o)=>f.text(saveBar(f.html,{page:'cameras',busy:false,t:tja,onSave(){},onReload(){},...o}));
test('the bar sticks to the bottom only while there is a draft (or a notice to read), and says so only while there is a draft',()=>{
 assert.equal(barSticky({dirty:false}),false);assert.equal(barSticky({dirty:true}),true);
 assert.equal(barSticky({dirty:false,notice:'読み直しました。'}),true);assert.equal(barSticky({dirty:false,applyNotice:'x'}),true);
 const f=fakeHTML();
 const clean=bar(f,{dirty:false});
 assert.ok(!clean.includes('保存していない変更があります'));assert.ok(!clean.includes('sticky'));
 assert.match(clean,/data-dirty=false/);
 const dirty=bar(f,{dirty:true});
 assert.ok(dirty.includes('class=save-bar sticky'));
 assert.match(dirty,/<p class="save-bar-msg" id=cameras-unsaved>保存していない変更があります<\/p>/);
 // the message, then 「保存する」, then 「読み直す」 (the same row for every group)
 assert.ok(dirty.indexOf('保存していない変更があります')<dirty.indexOf('>保存する<')&&dirty.indexOf('>保存する<')<dirty.indexOf('>読み直す<'));
 assert.match(dirty,/<button id=cameras-save type="button" class="btn primary" \?disabled=false @click=<fn#\d+>>保存する<\/button><button id=cameras-reload type="button" class="btn" \?disabled=false @click=<fn#\d+>>読み直す<\/button>/);
 assert.ok(!dirty.includes('再起動'));
 const en=f.text(saveBar(f.html,{page:'settings',dirty:true,busy:false,t:ten,onSave(){},onReload(){}}));
 assert.ok(en.includes('You have unsaved changes')&&en.includes('>Save<')&&en.includes('>Reload<'),en);
 assert.deepEqual(Object.keys(ui.ja).sort(),Object.keys(ui.en).sort());
});
test('the notices of the page are above the buttons of the row (the answer is where the person pressed)',()=>{
 const out=bar(fakeHTML(),{dirty:false,notice:'読み直しました。',applyNotice:'保存して反映しました。'});
 assert.ok(out.indexOf('読み直しました。')<out.indexOf('>保存する<'));
 assert.ok(out.indexOf('保存して反映しました。')<out.indexOf('>保存する<'));
 assert.match(out,/<p id=cameras-result class="notice" role="status">読み直しました。<\/p>/);
});
test('the restart is not in the save row: it is below a line',()=>{
 const f=fakeHTML(),out=f.text(restartZone(f.html,{page:'settings',busy:false,t:tja,onRestart(){}}));
 assert.match(out,/^<div class="restart-zone"><button id=settings-restart type="button" class="btn" \?disabled=false @click=<fn#0>>本体を再起動する<\/button><\/div>$/);
 const settings=read('app/settings.js');
 assert.ok(settings.indexOf('${saveBar(html,')<settings.indexOf('${restartZone(html,'));
 assert.ok(!settings.includes("['save','reload','restart'].map"));
 assert.match(settings,/onSave:e=>preview\('save',undefined,e\.currentTarget\)/);
 assert.match(settings,/onRestart:e=>preview\('restart',undefined,e\.currentTarget\)/);
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('.save-bar.sticky'),/position:sticky;bottom:0/);
 assert.match(body('.restart-zone'),/border-top:1px solid var\(--line\)/);
 // it is on the page of the cameras and of the settings: one mount draws both
 assert.match(settings,/export function mountSettings\(t,request,page='settings'\)/);
});
test('the button that makes a password again is under the field of that password, not in the row',()=>{
 const settings=read('app/settings.js');
 assert.match(settings,/const regenerate=page==='settings'\?\{'stream\.read_pass':'read','admin\.password':'admin'\}\[path\]:undefined;/);
 assert.match(settings,/<\/p><\/div>\$\{regenerate\?html`<button type="button" class="btn regenerate" data-regenerate=\$\{regenerate\}/);
 assert.ok(!settings.includes("(page==='cameras'?[]:s.group==='stream'?['read']"));
 assert.ok(ui.ja.settings_regenerate_read&&ui.ja.settings_regenerate_admin);
});
test('the switch of the automatic change is the first of its group',()=>{
 assert.match(read('app/settings.js'),/switch:\['auto_enabled','down_sec','fail_kbps','low_sec','recover_kbps','recover_sec'\]/);
});

// ---- the destination of a camera of the status page ----
test('the destination is a link of words only, at the right of the name of the camera, and 44px high',()=>{
 const main=read('app/main.js');
 assert.ok(main.includes('<a class="to-send" href="#cameras" @click=${e=>{e.preventDefault();goSender(c.id);}}>${t(\'sender_link\')}<span aria-hidden="true"> ›</span></a>'));
 assert.ok(!main.includes('btn to-send'));
 // it is in the row of the name (before the strip), not in the line under the strip
 assert.ok(main.indexOf('class="to-send"')<main.indexOf('class="strip"'));
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('.to-send'),/min-height: 44px/);
 assert.ok(!/border|background/.test(body('.to-send')));
 assert.match(body('.cam'),/grid-template-columns: minmax\(0,1fr\) auto auto/);
});

// ---- the operations of the status page ----
test('the heading 「注意」 is only there when there is something to say; the button that shows all is in its own part',()=>{
 const main=read('app/main.js');
 assert.match(main,/\$\{warnings\.length\|\|opsNotice\?html`<section data-tab="ops" id="warning-panel"><h2>\$\{t\('warning'\)\}<\/h2>/);
 assert.match(main,/<section data-tab="ops" id="display-panel"><h2>\$\{t\('camera_display'\)\}<\/h2>\$\{button\('cameras\/show-all',t\('show_all'\)\)\}<\/section>/);
 // the order: manual standby, warnings, show all, the stream
 assert.ok(main.indexOf("t('manual_standby')")<main.indexOf("t('warning')")&&main.indexOf("t('warning')")<main.indexOf("t('camera_display')")&&main.indexOf("t('camera_display')")<main.indexOf('class="stream-zone"'));
 assert.ok(!/<div id="warnings">\$\{\[\.\.\.new Set/.test(main));
 assert.equal(tja('camera_display'),'カメラの表示');assert.equal(ten('camera_display'),'Camera display');
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('#ops-panel>section'),/margin-bottom:24px/);
});
test('the error of an operation is shown by the operation; the others stay at the top',()=>{
 const main=read('app/main.js');
 assert.match(main,/notice=\(e,place=''\)=>update\(s=>\{setNotice\(s,e\);s\.noticePlace=place;\}\)/);
 assert.match(main,/const noticePlaceOf=path=>path==='standby\/off'\?'scenes':'ops';/);
 assert.match(main,/catch\(e\)\{notice\(e,noticePlaceOf\(path\)\);\}/);
 assert.match(main,/\['app-error',s=>html`\$\{s\.noticePlace\?'':s\.notice\}`\]/);
 // the 注意 part shows an error of the operations; the scene list shows an error of the scenes, at the top of its part
 assert.match(main,/opsNotice=s\.noticePlace==='ops'&&!!s\.notice/);
 assert.match(main,/<h2>\$\{t\('warning'\)\}<\/h2>\$\{opsNotice\?html`<p class="warn" role="alert">\$\{s\.notice\}<\/p>`:''\}/);
 assert.match(main,/<h2>\$\{t\('scenes'\)\}<\/h2>\$\{s\.noticePlace==='scenes'&&s\.notice\?html`<p class="warn" role="alert">\$\{s\.notice\}<\/p>`:''\}/);
});
test('the error of the login is in the card above the button, tied to the fields; the error of the source edit is by the heading of the body',()=>{
 const shell=read('app/shell.js');
 assert.ok(shell.indexOf('<h2 class="login-title">')<shell.indexOf('id="password"'));
 assert.ok(shell.indexOf('id="login-error"')<shell.indexOf('id="login-submit"'));
 assert.equal((shell.match(/aria-describedby="login-error" aria-invalid=\$\{s\.error\?'true':nothing\}/g)||[]).length,2);
 assert.match(shell,/<p id="login-error" class="warn" role="alert">\$\{s\.error\}<\/p>/);
 assert.match(shell,/render\(html`\$\{boot\.login\?'':s\.error\}`,document\.querySelector\('#error'\)\)/);
 const sources=read('app/sources.js');
 assert.ok(sources.indexOf('id="source-body-label">')<sources.indexOf('class="warn source-edit-error" role="alert">${s.editorError||s.previewError}')&&sources.indexOf('class="warn source-edit-error"')<sources.indexOf('id="source-body" class="source-body"'));
 assert.ok(!sources.includes('<p role="alert">${s.editorError||s.previewError}</p>'));
 // with the preview only, the error is still read
 assert.ok(flat.includes('.source-workspace.previewonly>.source-edit>:not(.source-edit-error){display:none;}')||flat.includes('.source-workspace.previewonly>.source-edit>:not(.source-edit-error){display:none}'));
});

// ---- the preview and the editor ----
test('the preview has the shape of the recommended size, stays in view beside a long column, and the editor has no empty space under it',()=>{
 assert.equal(previewRatio({obs_width:1920,obs_height:1080}),'1920 / 1080');
 assert.equal(previewRatio({obs_width:400,obs_height:100}),'400 / 100');
 for(const bad of [{},null,{obs_width:0,obs_height:100},{obs_width:'x',obs_height:5},{obs_width:-1,obs_height:-1},{obs_width:100}])assert.equal(previewRatio(bad),'16 / 9',JSON.stringify(bad));
 const sources=read('app/sources.js');
 assert.match(sources,/data-preview="editor" style=\$\{'aspect-ratio:'\+previewRatio\(s\.draft\)\}/);
 assert.match(sources,/data-preview=\$\{src\.name\} style=\$\{'aspect-ratio:'\+previewRatio\(src\)\}/);
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('.source-workspace > .source-preview'),/align-self:start/);
 assert.match(body('.source-workspace.horizontal > .source-preview'),/position:sticky; top:calc\(var\(--top-h\) \+ 12px\)/);
 assert.ok(!/min-height:\s*(220|420|300)px/.test(body('.source-preview')+body('.source-workspace.previewonly > .source-preview')));
 // the card of the list is a column of the flex: the preview there is not squeezed to its content
 assert.ok(!/align-self/.test(body('.source-preview')));
 assert.match(body('.source-body'),/display:flex; flex-direction:column/);
 assert.match(body('#sources-panel .cm-editor'),/flex:1 1 auto/);
});

// ---- the groups of the settings, the width of the fields ----
test('the groups of the settings are two by two on the phone; a number is narrow, a text is not endless',()=>{
 assert.match(flat,/@media\(max-width:899px\)\{\.settings-layout\{[^}]*\}#settings-groups\{[^}]*\}#settings-groupsbutton\{flex:1045%\}\}/);
 assert.ok(!flat.includes('flex:1030%'));
 assert.ok(flat.includes('.settings-field>span:not(.password-control)>input[type=number]{max-width:12em}'));
 assert.ok(flat.includes('box-sizing:border-box;max-width:40em}'));
 assert.ok(flat.includes('.settings-field.password-control{max-width:40em}'));
});
test('small things: the red button is apart from the details, the state of a camera card is a tag, the mark of the menu is apart, the login card has a heading',()=>{
 const body=selector=>rules.filter(r=>r.selector===selector).map(r=>r.body).join(';');
 assert.match(body('.settings-card>.btn.stop'),/margin-top:16px/);
 assert.match(body('.settings-card>details'),/margin-top:12px/);
 assert.match(body('.sheet-nav small,#menu small'),/margin-left:8px/);
 assert.match(body('.sheet-nav small,#menu small'),/color:var\(--low\)/);
 assert.match(read('app/settings.js'),/<p class="card-state" data-state=\$\{[^}]*\}><span class="state">/);
 assert.match(body('.card-state[data-state="LOW"] .state'),/var\(--low\)/);
 assert.match(body('.source-test-title'),/flex:0 0 100%/);
 assert.match(read('app/sources.js'),/<div class="source-page-head"><h2>\$\{t\('sources'\)\}<\/h2>\$\{s\.draft\?'':html`<button class="btn primary" @click=\$\{\(\)=>open\(\)\}>\$\{t\('src_new'\)\}<\/button>`\}<\/div>/);
});

// ---- the phone window and the address of the camera page ----
test('the phone window: the buttons are .btn, the fields are .settings-field with the name above, the direct input is a URL',()=>{
 const phone=read('app/phone.js');
 assert.match(phone,/<div class="settings-field"><label for="phone-address">\$\{t\('phone_address'\)\}<\/label><select id="phone-address"/);
 assert.match(phone,/<div class="settings-field" \?hidden=\$\{s\.address!==''\}><label for="phone-direct">\$\{t\('settings_direct'\)\}<\/label><input id="phone-direct" type="text" inputmode="url"/);
 assert.match(phone,/<button id="phone-create" type="button" class="btn primary"/);
 assert.match(phone,/<button id="phone-close" type="button" class="btn"/);
 assert.ok(!/<input id="phone-direct" aria-label/.test(phone));
});
test('the address of the destination is a field of the settings form, and its notes are .warn',()=>{
 const main=read('app/main.js');
 assert.match(main,/<div class="settings-field"><label for="camera-address-select">\$\{t\('sender_address'\)\}<\/label><select id="camera-address-select"/);
 assert.match(main,/<p class="warn">\$\{s\.senderNotice\}<\/p>/);
 assert.match(main,/\.map\(m=>html`<p class="warn">\$\{m\}<\/p>`\)/);
});

// ---- the inputs: number fields, empty ids ----
test('a number field has no role of its own, and its min and max go together',()=>{
 const settings=read('app/settings.js');
 assert.ok(!settings.includes("'textbox'"));
 assert.match(settings,/role=\$\{bool\?'switch':nothing\}/);
 assert.match(settings,/min=\$\{bounds\.min\?\?nothing\} max=\$\{bounds\.max\?\?nothing\}/);
 assert.deepEqual(numberBounds('admin.port','port'),{min:1,max:65535});
 assert.deepEqual(numberBounds('mediamtx.rtmp_port','rtmp_port'),{min:1,max:65535});
 assert.deepEqual(numberBounds('stream.publish_rtmp_port','publish_rtmp_port'),{min:0,max:65535});
 assert.deepEqual(numberBounds('log.keep_days','keep_days'),{min:1,max:3650});
 assert.deepEqual(numberBounds('switch.low_sec','low_sec'),{min:1,max:86400});
 assert.deepEqual(numberBounds('cameras[cam1].switch.down_sec','down_sec'),{min:1,max:86400});
 assert.deepEqual(numberBounds('switch.fail_kbps','fail_kbps'),{min:0,max:1000000});
 assert.deepEqual(numberBounds('x.y','y'),{});
 // every number of the table that has a bound has both
 for(const [path,key] of [['switch.recover_kbps','recover_kbps'],['switch.recover_sec','recover_sec'],['mediamtx.api_port','api_port']]){const b=numberBounds(path,key);assert.ok(b.min!==undefined&&b.max!==undefined,path);}
});
test('no attribute is given an empty id',()=>{
 for(const file of ['app/main.js','app/settings.js','app/shell.js','app/sources.js','app/phone.js'])assert.ok(!/id=\$\{[^}]*\|\|''\}/.test(read(file)),file);
 assert.match(read('app/main.js'),/\[path\]\|\|nothing\}/);
});

// ---- the words of the windows ----
const commonRun=(a,b)=>{let best=0;const row=new Array(b.length+1).fill(0);for(let i=1;i<=a.length;i++){for(let j=b.length;j>=1;j--){row[j]=a[i-1]===b[j-1]?row[j-1]+1:0;if(row[j]>best)best=row[j];}row[0]=0;}return best;};
const contentWords=text=>new Set(text.toLowerCase().replace(/[^a-z0-9’' ]/g,' ').split(/\s+/).filter(w=>w&&!['the','a','an','of','to','will','be','is','are','by','that','this','in','on','it','you','as'].includes(w)).map(w=>w.replace(/(ing|es|s|e|ed)$/,'')));
test('the window of the stream says what happens, not the title again',()=>{
 for(const [lang,t] of [['ja',tja],['en',ten]])for(const [path,scene] of [['stream/start',''],['stream/start','配信開始'],['stream/stop','']]){
  const spec=streamConfirm(path,t,scene);
  for(const line of spec.lines){
   if(lang==='ja')assert.ok(commonRun(spec.title,line)<6,`${spec.title} / ${line}`);
   else assert.ok([...contentWords(line)].filter(w=>contentWords(spec.title).has(w)).length<2,`${spec.title} / ${line}`);
  }
 }
 assert.deepEqual(streamConfirm('stream/start',tja).lines,[]);
 assert.deepEqual(streamConfirm('stream/start',tja,'配信開始').lines,['OBS は「配信開始」のシーンに切り替わります。']);
 assert.deepEqual(streamConfirm('stream/stop',tja).lines,['視聴者の映像が切れます。']);
 assert.deepEqual(streamConfirm('stream/stop',ten).lines,['Viewers will lose the picture.']);
 assert.equal(ui.ja.stream_start_note,undefined);assert.equal(ui.en.stream_start_note,undefined);
});
const saved={cameras:[{id:'cam1',name:''},{id:'cam2',name:'サブ'}]};
test('creating a password again says 「作り直すと」, not 「保存すると」; the save keeps its words',()=>{
 for(const [lang,t,restart,immediate,save] of [['ja',tja,'作り直すと、本体が再起動します。','作り直すと、すぐに反映します。','保存すると、本体が再起動します。'],['en',ten,'Creating it again restarts the app.','Creating it again applies at once.','Saving restarts the app.']]){
  for(const target of ['read','publish']){
   const lines=confirmationLines('regenerate',{target,camera:'cam1'},{apply:'restart'},false,t,saved);
   assert.ok(lines.includes(restart),lang+' '+target);assert.ok(!lines.includes(save),lang+' '+target);
  }
  const admin=confirmationLines('regenerate',{target:'admin'},{apply:'immediate'},false,t,saved);
  assert.ok(admin.includes(immediate),lang);assert.ok(!admin.includes(t('settings_immediate')),lang);
  assert.ok(confirmationLines('save',{values:{}},{apply:'restart',fields:[{path:'admin.port'}]},false,t,saved).includes(save),lang);
  assert.ok(confirmationLines('save',{values:{}},{apply:'immediate',fields:[{path:'log.keep_days'}]},false,t,saved).includes(t('settings_immediate')),lang);
 }
});
test('creating the sending password again while streaming has one line about the reception, not two',()=>{
 for(const [lang,t] of [['ja',tja],['en',ten]]){
  const lines=confirmationLines('regenerate',{target:'publish',camera:'cam1'},{apply:'restart'},true,t,saved);
  assert.ok(lines.includes(t('settings_stream_warning')),lang);
  assert.ok(!lines.includes(t('publish_reset_warning')),lang);
  // not streaming: the line about the restart of MediaMTX stays (there is no stream warning)
  const idle=confirmationLines('regenerate',{target:'publish',camera:'cam1'},{apply:'restart'},false,t,saved);
  assert.ok(idle.includes(t('publish_reset_warning'))&&!idle.includes(t('settings_stream_warning')),lang);
 }
 const ja=confirmationSpec('regenerate',{target:'publish',camera:'cam1'},{apply:'restart'},true,tja,saved);
 assert.equal(ja.lines.filter(l=>l.includes('受信')).length,1);
});
test('both tables have the same keys and the new ones are texts',()=>{
 for(const key of ['settings_unsaved_bar','settings_regenerate_restart_mode','settings_regenerate_immediate','camera_display']){assert.ok(ui.ja[key]&&ui.en[key],key);}
 assert.deepEqual(Object.keys(ui.ja).sort(),Object.keys(ui.en).sort());
});
