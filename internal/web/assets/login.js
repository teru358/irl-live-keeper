'use strict';
const boot = JSON.parse(document.querySelector('#bootstrap').textContent);
function selectLanguage(saved, config, languages) {
  if (saved === 'ja' || saved === 'en') return saved;
  if (config === 'ja' || config === 'en') return config;
  return (languages[0] || '').toLowerCase().split('-')[0] === 'ja' ? 'ja' : 'en';
}
let saved; try { saved = localStorage.getItem('language'); } catch (_) {}
let lang = selectLanguage(saved, boot.language, navigator.languages || [navigator.language]);
const t = key => boot.catalogs[lang][key];
document.querySelectorAll('[data-language]').forEach(b => { b.setAttribute('aria-pressed',String(b.dataset.language===lang)); b.addEventListener('click', () => { const next=b.dataset.language; try { localStorage.setItem('language', next); } catch (_) {} location.reload(); }); });
const el = id => document.getElementById(id);
const text = (id, value) => { el(id).textContent = value; };
document.documentElement.lang = lang; document.title = t('title');
document.querySelectorAll('[data-ui]').forEach(e => e.textContent = t(e.dataset.ui));
function applyTheme(value) {
 if(value==='light'||value==='dark') document.documentElement.dataset.theme=value; else delete document.documentElement.dataset.theme;
 document.querySelectorAll('[data-theme-set]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.themeSet===value)));
}
function applyTab(value) {
 if(!['line','scene','ops'].includes(value)) value='line';
 document.body.dataset.tab=value;
 document.querySelectorAll('[data-tab-set]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.tabSet===value)));
}
let theme='system', tab='line';try{theme=localStorage.getItem('theme')||theme;tab=localStorage.getItem('tab')||tab;}catch(_){}
applyTheme(theme);applyTab(tab);
document.querySelectorAll('[data-theme-set]').forEach(b=>b.onclick=()=>{applyTheme(b.dataset.themeSet);try{localStorage.setItem('theme',b.dataset.themeSet);}catch(_){}});
document.querySelectorAll('[data-tab-set]').forEach(b=>b.onclick=()=>{applyTab(b.dataset.tabSet);try{localStorage.setItem('tab',b.dataset.tabSet);}catch(_){}});
const menuButton=document.querySelector('.menu-btn');
const prefs=document.querySelector('.sheet-prefs'), desktop=matchMedia('(min-width:900px)');
function placePreferences(){
 (desktop.matches?el('desktop-prefs'):el('sheet')).append(prefs);
 (desktop.matches?el('menu'):el('sheet')).append(el('logout'));
}
placePreferences();desktop.addEventListener('change',placePreferences);
menuButton.onclick=()=>{const open=el('sheet').classList.toggle('open');menuButton.setAttribute('aria-expanded',String(open));};
if(boot.login){el('tally').hidden=true;el('tabs').hidden=true;el('summary').hidden=true;el('logout').hidden=true;}
async function request(path, body, method) {
 const r = await fetch('/api/' + path + (path.includes('?') ? '&' : '?') + 'lang=' + lang, {method: method || (body === undefined ? 'GET' : 'POST'), headers: {'Content-Type':'application/json','X-IRL-Live-Keeper':'1'}, body: body === undefined ? undefined : JSON.stringify(body)});
 if(r.status === 401 && !boot.login) location.replace('/?login');
 const data = await r.json(); if (!r.ok) {const error=Error(data.error || t('failed'));error.code=data.code;error.fields=data.fields;error.status=r.status;throw error;} return data;
}
if (boot.login) el('login').hidden = false; el('admin').hidden = boot.login;
function passwordEye(input, button) {
 const wrap=document.createElement('span');wrap.className='password-control';input.replaceWith(wrap);wrap.append(input);
 const eye=button||document.createElement('button');eye.type='button';eye.className='password-eye';
 eye.innerHTML='<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="2"/><path class="eye-slash" d="m3 3 18 18" stroke="currentColor" stroke-width="2"/></svg>';
 const update=()=>{const visible=input.type==='text';eye.setAttribute('aria-label',t(visible?'sender_hide':'sender_show'));eye.setAttribute('aria-pressed',String(visible));};
 eye.onclick=()=>{input.type=input.type==='password'?'text':'password';update();};wrap.append(eye);update();
}
if(boot.login)passwordEye(el('password'),el('setup-show'));
if(boot.setup){el('setup-fields').hidden=false;el('password').minLength=8;el('password').autocomplete='new-password';el('password-confirm').required=true;el('password-confirm').minLength=8;el('login-submit').textContent=t('setup_title');passwordEye(el('password-confirm'));}
el('login')?.addEventListener('submit', async e => { e.preventDefault(); try { if(boot.setup && el('password').value!==el('password-confirm').value)throw Error(t('setup_mismatch')); await request(boot.setup?'setup':'login', {password:el('password').value}); location.replace('/'); } catch(e) { text('error',e.message); } });
if (!boot.login) {
 el('logout').onclick=async()=>{try{await request('logout',{});location.replace('/?login');}catch(e){text('error',e.message);}};
 document.querySelectorAll('[data-page]').forEach(b => b.onclick = () => { document.body.dataset.page=b.dataset.page;el('sheet').classList.remove('open');menuButton.setAttribute('aria-expanded','false');document.querySelectorAll('[data-page]').forEach(x=>{if(x.dataset.page===b.dataset.page)x.setAttribute('aria-current','page');else x.removeAttribute('aria-current');});const isLogs=b.dataset.page==='logs'; el('settings-panel').hidden=b.dataset.page!=='settings'; if(b.dataset.page==='settings'&&!settingsData)loadSettings(); el('status').hidden = b.dataset.page !== 'status'; el('log-panel').hidden=!isLogs; el('sender-panel').hidden=b.dataset.page!=='sender'; el('soon').hidden = b.dataset.page==='status'||isLogs||b.dataset.page==='sender'||b.dataset.page==='settings'; if(b.dataset.page==='sender') loadSender(); if(isLogs) loadLogs(); });
 document.querySelectorAll('[data-command]').forEach(b => b.onclick = async () => {
  const path = b.dataset.command;
  if(path === 'stream/stop' && !confirm(t('confirm_stop'))) return;
  if(path === 'cameras/show-all' && !confirm(t('confirm_show'))) return;
  b.disabled = true; try { await request(path, {}); text('error',''); } catch(e) { text('error',e.message); } finally { b.disabled = false; }
 });
 el('phone-direct').setAttribute('aria-label',t('sender_address'));
 el('phone-address').onchange=()=>{el('phone-direct').hidden=el('phone-address').value!=='';};
 let phoneTimer;
 document.querySelectorAll('[data-phone]').forEach(button=>{button.hidden=!boot.local||!desktop.matches;desktop.addEventListener('change',()=>{button.hidden=!boot.local||!desktop.matches;});button.onclick=async()=>{el('phone-dialog').showModal();try{const settings=await request('settings');el('phone-controls').hidden=!settings.values.admin.lan;el('phone-disabled').hidden=settings.values.admin.lan;if(!settings.values.admin.lan)return;const sender=await request('sender');el('phone-address').replaceChildren(...sender.addresses.map(a=>{const o=document.createElement('option');o.value=a.address;o.textContent=t('sender_'+a.kind)+' '+a.address;return o;}));const direct=document.createElement('option');direct.value='';direct.textContent=t('settings_direct');el('phone-address').append(direct);el('phone-address').value=sender.default;el('phone-direct').hidden=true;}catch(e){text('phone-result',e.message);}};});
 el('phone-close').onclick=()=>{clearTimeout(phoneTimer);el('phone-qr').replaceChildren();text('phone-url','');el('phone-dialog').close();};
 el('phone-create').onclick=async()=>{clearTimeout(phoneTimer);el('phone-qr').replaceChildren();text('phone-url','');try{const result=await request('phone-link',{address:el('phone-address').value||el('phone-direct').value});const code=qrcode(0,'M');code.addData(result.url);code.make();const svg=new DOMParser().parseFromString(code.createSvgTag({cellSize:4,margin:16,scalable:true}),'image/svg+xml').documentElement;el('phone-qr').append(document.importNode(svg,true));text('phone-url',result.url.replace(/t=[^&]+/,'t=••••••'));text('phone-result',t('phone_lifetime'));phoneTimer=setTimeout(()=>{el('phone-qr').replaceChildren();text('phone-url','');text('phone-result',t('phone_expired'));},60000);}catch(e){text('phone-result',e.message);}};
 let settingsBaseline, settingsApplyKey, settingsRefreshPending=false;
 let settingsData, settingsValues, settingsGroup='admin', settingsBusy=false, settingsAction, settingsPayload, settingsStreaming=false, settingsOBS=false, settingsScenes=[];
 const fieldErrors=new Map();
 const settingsMode=p=>settingsData?.apply_modes?.[p.replace(/\[\d+\]/g,'[]')]||'restart';
 function settingNode(tag,value,cls){const n=document.createElement(tag);if(value)n.textContent=value;if(cls)n.className=cls;return n;}
 async function loadSettings(){text('error','');text('settings-result','');settingsData=undefined;try{settingsData=await request('settings');settingsValues=structuredClone(settingsData.values);settingsBaseline=JSON.stringify(settingsValues);settingsScenes=[];if(settingsOBS){try{settingsScenes=await request('obs/scenes');}catch(_){}}drawSettings();settingsBaseline=JSON.stringify(settingsValues);}catch(e){settingsError(e);}}
 function friendlyFieldError(message){return message.replace(/[a-z_]+(?:\[\d+\])?(?:\.[a-z_]+)+/g,path=>path.split('.').map(k=>t('setting_'+k.replace(/\[\d+\]/g,''))||t('settings')).join(' / '));}
 const settingOrders={stream:['app','obs_read_host','publish_host','publish_rtmp_port','publish_srt_port','publish_user','publish_pass','read_user','read_pass'],obs:['host','port','password','standby_scene','scene_fallbacks','studio_mode_hide'],switch:['enabled','down_sec','fail_kbps','low_sec','recover_kbps','recover_sec']};
 function settingsError(e){const message=t('settings_not_saved')+' '+e.message;text('error',message);text('settings-result',message);for(const [p,msg] of fieldErrors)msg.textContent=friendlyFieldError(e.fields?.[p]||'');if(e.fields){const first=Object.keys(e.fields)[0];const input=document.querySelector('[data-setting-path="'+CSS.escape(first)+'"]');if(input){settingsGroup=input.dataset.group;drawSettings();for(const [p,msg] of fieldErrors)msg.textContent=friendlyFieldError(e.fields[p]||'');const target=document.querySelector('[data-setting-path="'+CSS.escape(first)+'"]');if(target?.closest('details'))target.closest('details').open=true;target?.focus();document.querySelector('[data-setting-path="'+CSS.escape(first)+'"]')?.scrollIntoView({block:'center'});}}}
 function drawSettings(){fieldErrors.clear();el('settings-groups').replaceChildren();el('settings-fields').replaceChildren();const groups={camera:['cameras','switch','on_stream_start'],stream:['stream'],obs:['obs'],admin:['admin','mediamtx'],other:['gui','log','language']};for(const [group,keys] of Object.entries(groups)){const b=settingNode('button',t('settings_group_'+group));b.type='button';b.setAttribute('aria-pressed',String(group===settingsGroup));b.onclick=()=>{settingsGroup=group;drawSettings();};el('settings-groups').append(b);const section=settingNode('div');section.hidden=group!==settingsGroup;for(const key of keys){section.append(settingNode('h3',t('setting_'+key)));drawSetting(section,settingsValues,key,key,group);}section.append(settingNode('p',t('settings_restart_note'),'settings-note'));el('settings-fields').append(section);}

 el('settings-local').hidden=settingsData.local;el('settings-lan').hidden=true;}
 function drawSetting(parent,obj,key,path,group){if(obj[key]===null&&['cameras','scene_fallbacks'].includes(key))obj[key]=[];const value=obj[key];
 if(Array.isArray(value)){if(key==='scene_fallbacks')parent.append(settingNode('h4',t('setting_'+key)),settingNode('p',t('setting_'+key+'_help')));value.forEach((v,i)=>{const card=settingNode(path==='cameras'?'details':'fieldset',null,'settings-card');card.append(settingNode(path==='cameras'?'summary':'legend',path==='cameras'?v.name+' ('+v.id+')':t('setting_'+key)+' '+(i+1)));drawSetting(card,value,i,path+'['+i+']',group);const remove=settingNode('button',t('settings_remove'),'btn');remove.type='button';remove.onclick=()=>{value.splice(i,1);drawSettings();};card.append(remove);parent.append(card);});const add=settingNode('button',t('settings_add'),'btn');add.type='button';add.onclick=()=>{value.push(path==='cameras'?{id:'',name:'',on_bad:'standby',switch:null}:{scene:settingsScenes[0]?.name||'',fallback:settingsScenes[0]?.name||''});drawSettings();};parent.append(add);return;}
 if(value!==null&&typeof value==='object'){for(const k of [...new Set([...(settingOrders[key]||[]).filter(k=>k in value),...Object.keys(value)])])drawSetting(parent,value,k,path+'.'+k,group);return;}
 if(value===null && key==='switch'){obj[key]={fail_kbps:null,low_sec:null,recover_kbps:null,recover_sec:null,down_sec:null};drawSetting(parent,obj,key,path,group);return;}
 const row=settingNode('div',null,'settings-field');row.hidden=path==='on_stream_start.scene'&&!settingsValues.on_stream_start.enabled;
 const label=settingNode('label',t('setting_'+key));const id='setting-'+path;label.htmlFor=id;const badge=settingNode('small',settingsMode(path)==='restart'?'＊':'','settings-badge');label.append(badge);row.append(label,settingNode('small',t('setting_'+key+'_help'),'settings-help'));
 const choices={language:['auto','ja','en'],on_bad:['hide','standby'],studio_mode_hide:['standby','ignore','hide'],close_action:['minimize','confirm','quit']};
 const bool=typeof value==='boolean', number=typeof value==='number'||(value===null&&path.includes('.switch.')), secret=/password|_pass$/.test(key);
 const localOnly=!settingsData.local&&['mediamtx.binary','log.dir'].includes(path);
 const input=bool?el('settings-switch-template').content.firstElementChild.cloneNode(true):localOnly?el('settings-local-template').content.firstElementChild.cloneNode(true):document.createElement(choices[key]?'select':'input');input.id=id;input.dataset.settingPath=path;input.dataset.group=group;
 if(choices[key]){for(const v of choices[key]){const o=settingNode('option',t('setting_option_'+v));o.value=v;input.append(o);}input.value=value;}else{input.type=bool?'checkbox':secret?'password':number?'number':'text';if(bool){input.checked=value;input.setAttribute('role','switch');}else input.value=value??'';if(secret)input.autocomplete='new-password';}
 if(number){input.removeAttribute('min');input.removeAttribute('max');if(key==='port'||key.endsWith('_port')){input.min=key.startsWith('publish_')?'0':'1';input.max='65535';}if(path==='log.keep_days')input.min='1';}
 const inheritedPort={publish_rtmp_port:'rtmp_port',publish_srt_port:'srt_port'}[key];
 if(inheritedPort){input.placeholder=t('settings_same_port').replace('{name}',t('setting_'+inheritedPort)).replace('{port}',settingsValues.mediamtx[inheritedPort]);if(value===0)input.value='';}
 if(['mediamtx.binary','stream.obs_read_host','stream.publish_host'].includes(path))input.placeholder=t('settings_default_'+key);
 if(value===null&&path.includes('.switch.'))input.placeholder=String(settingsValues.switch[key]);
 input.readOnly=localOnly;input.onchange=()=>{if(path==='admin.lan'&&input.checked&&!confirm(t('settings_lan'))){input.checked=false;return;}obj[key]=bool?input.checked:number?(input.value===''?(path.includes('.switch.')?null:0):Number(input.value)):input.value;if(path==='on_stream_start.enabled')drawSettings();};row.append(input);if(localOnly)row.append(settingNode('small',t('settings_local')));
 if(secret)passwordEye(input);
 if(path==='admin.lan')row.append(settingNode('p',t('settings_lan_notice'),'warn'));
 if(['scene','fallback','standby_scene'].includes(key)){if(settingsScenes.length){const select=document.createElement('select');select.setAttribute('aria-label',t('setting_'+key));for(const scene of settingsScenes){const o=settingNode('option',scene.name);o.value=scene.name;select.append(o);}const direct=settingNode('option',t('settings_direct'));direct.value='';select.append(direct);select.value=settingsScenes.some(s=>s.name===value)?value:'';input.hidden=select.value!=='';select.onchange=()=>{input.hidden=select.value!=='';if(select.value){input.value=select.value;input.onchange();}else input.focus();};row.insertBefore(select,input);const missing=settingNode('small',t('settings_scene_missing'));row.append(missing);const updateMissing=()=>{missing.hidden=!input.value||settingsScenes.some(s=>s.name===input.value);};input.addEventListener('change',updateMissing);select.addEventListener('change',updateMissing);updateMissing();}else row.append(settingNode('small',t('settings_disconnected')));}

 const error=settingNode('p',null,'warn');error.id=id+'-error';error.setAttribute('role','alert');input.setAttribute('aria-describedby',error.id);fieldErrors.set(path,error);row.append(error);parent.append(row);
 }
 function settingsLock(on){settingsBusy=on;for(const id of ['settings-save','settings-revert','settings-restart','settings-accept','settings-reload'])el(id).disabled=on;document.querySelectorAll('[data-regenerate]').forEach(b=>b.disabled=on);}
 async function previewSettings(action,payload){if(settingsBusy)return;settingsLock(true);try{if(!settingsData)await loadSettings();if(!settingsData)return;settingsPayload=payload||{values:structuredClone(settingsValues),hash:settingsData.hash};settingsAction=action;let preview;
 if(action==='restart')preview={apply:'restart'};else if(action==='regenerate')preview={apply:settingsPayload.target==='admin'?'immediate':'restart',obs_url_changes:settingsPayload.target==='read'};else preview=await request('settings/preview',action==='revert'?{revert:true}:settingsPayload);
 const box=el('settings-preview');box.replaceChildren();if(action==='regenerate')box.append(settingNode('p',t('settings_regenerate_'+settingsPayload.target)));box.classList.toggle('danger',settingsStreaming);box.append(settingNode('p',t(preview.apply==='restart'?'settings_restart_mode':'settings_immediate')));if(settingsStreaming&&preview.apply==='restart')box.append(settingNode('p',t('settings_stream_warning')));for(const f of preview.fields||[]){const key=f.path.replace(/\[\d+\]/g,'').split('.').pop();const camera=f.path.match(/^cameras\[(\d+)\]\.(.+)$/);const fieldLabel=t('setting_'+key);const label=camera?t('setting_cameras')+' '+(Number(camera[1])+1)+' — '+fieldLabel:fieldLabel;box.append(settingNode('p',label));}if(preview.admin_lan_will_close)box.append(settingNode('p',t('settings_lan_close')));for(const f of preview.fields||[])if(f.local_only&&!settingsData.local)box.append(settingNode('p',t('settings_local')));
 el('settings-update-label').hidden=!preview.obs_url_changes;el('settings-update').checked=false;el('settings-confirm').showModal();}catch(e){settingsError(e);}finally{settingsLock(false);}}
 el('settings-save').onclick=()=>previewSettings('save');el('settings-reload').onclick=async()=>{await loadSettings();if(settingsData)text('settings-result',t('settings_reloaded'));};el('settings-revert').onclick=()=>previewSettings('revert');el('settings-restart').onclick=()=>previewSettings('restart');el('settings-cancel').onclick=()=>el('settings-confirm').close();
 document.querySelectorAll('[data-regenerate]').forEach(b=>b.onclick=async()=>{if(settingsData&&JSON.stringify(settingsValues)!==settingsBaseline){text('settings-result',t('settings_changed'));return;}await loadSettings();if(settingsData)previewSettings('regenerate',{hash:settingsData.hash,target:b.dataset.regenerate});});
 el('settings-accept').onclick=async()=>{if(settingsBusy)return;settingsLock(true);try{const action=settingsAction;const payload={...settingsPayload,update_obs_input:el('settings-update').checked};const submitted=JSON.stringify(settingsValues);const result=await request(action==='restart'?'restart':action==='save'?'settings':'settings/'+action,action==='restart'?{}:action==='revert'?{update_obs_input:payload.update_obs_input}:payload,action==='save'?'PUT':'POST');settingsBaseline=submitted;settingsRefreshPending=true;el('settings-confirm').close();text('error','');text('settings-result',t(action==='revert'?'settings_undone':'settings_saved'));if(!el('settings-update-label').hidden&&!payload.update_obs_input)text('settings-result',t('settings_pending_urls'));if(result.apply==='restart'||action==='restart'){restarting=true;since=Date.now();connection();if(result.admin_url)setTimeout(()=>location.assign(result.admin_url),1500);}else{await loadSettings();text('settings-result',t(action==='revert'?'settings_undone':'settings_applied'));if(!el('sender-panel').hidden)loadSender();}}catch(e){el('settings-confirm').close();settingsError(e);}finally{settingsLock(false);}};

 let senderData, senderCamera='', senderHost='';
 const senderProtocols=new Map();
 async function loadSender(){try{senderData=await request('sender');let savedHost;try{savedHost=localStorage.getItem('sender-address');}catch(_){} senderHost=senderData.addresses.some(a=>a.address===savedHost)?savedHost:senderData.default;
 el('sender-address').replaceChildren(...senderData.addresses.map(a=>{const o=document.createElement('option');o.value=a.address;o.textContent=t('sender_'+a.kind)+' '+a.address;return o;}));el('sender-address').value=senderHost;drawSender();}catch(e){text('error',e.message);}}
 el('sender-address').onchange=()=>{senderCamera=el('sender-cameras').querySelector('details[open]')?.dataset.camera||senderCamera;const y=window.scrollY;senderHost=el('sender-address').value;try{localStorage.setItem('sender-address',senderHost);}catch(_){}drawSender(y);};
 function drawSender(restoreScroll){const a=senderData.addresses.find(a=>a.address===senderHost);if(!a)return;
 if(senderCamera&&!a.cameras.some(c=>c.id===senderCamera))senderCamera='';el('sender-cameras').replaceChildren(...a.cameras.map((c,index)=>{const card=document.createElement('details');card.className='send-cam';card.dataset.camera=c.id;card.open=senderCamera?c.id===senderCamera:index===0;const title=document.createElement('summary');title.textContent=c.name+' ('+c.id+')';card.append(title);
 const tabs=document.createElement('div');tabs.className='seg';tabs.setAttribute('role','group');tabs.setAttribute('aria-label',t('sender_protocol'));card.append(tabs);
 const fields=document.createElement('div');card.append(fields);
 let protocol=senderProtocols.get(c.id);if(!c[protocol])protocol=c.rtmp?'rtmp':'srt';
 const draw=()=>{fields.replaceChildren();tabs.querySelectorAll('button').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.protocol===protocol)));const p=c[protocol];if(p){
 const all=settingNode('button',t('sender_copy_all'),'btn primary');all.type='button';all.onclick=async()=>{if(await copySenderValue(p.url))all.textContent=t('sender_copied');};fields.append(all);
 const table=settingNode('table',null,'sender-table');fields.append(table);
 const port=new URL(p.url).port;senderField(table,t('sender_port'),port,false);
 for(const key of ['address',protocol==='srt'?'streamid':'key','user','password'])senderField(table,t({url:'sender_url',address:'sender_endpoint',streamid:'sender_streamid',key:'sender_key',user:'sender_user',password:'sender_password'}[key]),p[key],key==='password'||key==='url'||key==='streamid'||key==='key');
 if(protocol==='rtmp'){const hint=document.createElement('p');hint.className='hint';hint.textContent=t('sender_rtmp_hint');fields.append(hint);}}
 const obsTable=settingNode('table',null,'sender-table');fields.append(obsTable);senderField(obsTable,t('sender_obs'),c.obs,true);
};
 for(const name of ['rtmp','srt'])if(c[name]){const b=document.createElement('button');b.type='button';b.dataset.protocol=name;b.textContent=name.toUpperCase();b.onclick=()=>{protocol=name;senderProtocols.set(c.id,name);draw();};tabs.append(b);}draw();return card;}));
 const open=el('sender-cameras').querySelector('details[open]');if(open)senderCamera=open.dataset.camera;if(restoreScroll!==undefined)requestAnimationFrame(()=>window.scrollTo(0,restoreScroll));
 }
 function senderField(parent,label,value,secret){const row=document.createElement(parent.tagName==='TABLE'?'tr':'div');row.className='sender-field';const name=document.createElement(parent.tagName==='TABLE'?'th':'label');name.textContent=label;const input=document.createElement('input');input.readOnly=true;input.value=value||'';input.type=secret?'password':'text';input.setAttribute('aria-label',label);row.append(name);const cell=document.createElement(parent.tagName==='TABLE'?'td':'div');cell.append(input);row.append(cell);
 if(secret)passwordEye(input);
 const copy=document.createElement('button');copy.type='button';copy.className='copy';copy.textContent=t('sender_copy');copy.onclick=async()=>{if(await copySenderValue(value||'')){copy.textContent=t('sender_copied');setTimeout(()=>copy.textContent=t('sender_copy'),2500);}else{input.type='text';input.focus();input.select();input.setSelectionRange(0,input.value.length);text('sender-notice',t('sender_manual'));}};cell.append(copy);parent.append(row);}
 let restarting = false, since = 0;
 function connection() { text('connection', restarting ? t('restarting') + (Date.now()-since >= 30000 ? ' ' + t('restart_help') : '') : t('disconnected')); }
 let sceneList=[], currentScene='', pendingScene='', sceneTimer, returnScene='';
 const readings=new Map(), states=new Map(), cameraNames=new Map();
 function drawScenes(){if(!wasConnected){text('scenes',t('scenes_disconnected'));return;}el('scenes').replaceChildren(...sceneList.map(s=>{const b=document.createElement('button');const current=s.uuid===currentScene;b.className='scene k-'+(s.kind==='camera'?'cam':s.kind||'none');
 const add=(tag,value,cls)=>{const e=document.createElement(tag);e.textContent=value;if(cls)e.className=cls;b.append(e);};
 add('span',t('kind_'+(s.kind||'none')),'kind');add('span',s.name);
 if(s.cameras?.length)add('small',s.cameras.map(id=>cameraNames.get(id)||id).join(lang==='ja'?'＋':' + '));
 if(s.fallback_of?.length)add('i',t('fallback_of').replace('{scenes}',s.fallback_of.join('、')));
 if(current)add('em',t('current_scene'));if(s.name===returnScene)add('em',t('return_scene'));b.setAttribute('aria-pressed',String(current));b.disabled=!!pendingScene;b.onclick=async()=>{pendingScene=s.uuid;drawScenes();sceneTimer=setTimeout(()=>{pendingScene='';drawScenes();text('error',t('failed'));},10000);try{await request('obs/scene',{scene_uuid:s.uuid});}catch(e){clearTimeout(sceneTimer);pendingScene='';drawScenes();if(e.code!=='api.disconnected')text('error',e.message);}};return b;}));}
 let loadingScenes=false, scenesLoadedAt=0;
 async function loadScenes(){if(loadingScenes)return;loadingScenes=true;scenesLoadedAt=Date.now();try{const list=await request('obs/scenes');if(wasConnected)sceneList=list;drawScenes();text('error','');}catch(e){if(e.code!=='api.disconnected')text('error',e.message);}finally{loadingScenes=false;}}
 setInterval(()=>{if(wasConnected)loadScenes();},10000);
 let wasConnected=false;
 function render(s) {
 settingsStreaming=!!s.obs.streaming;settingsOBS=!!s.obs.connected;
 const applyKey=JSON.stringify([s.generation,s.apply]);if(settingsApplyKey!==applyKey){const previous=settingsApplyKey;settingsApplyKey=applyKey;if(settingsData&&(previous!==undefined||settingsRefreshPending)&&['applied','reverted'].includes(s.apply?.state)){settingsRefreshPending=false;if(JSON.stringify(settingsValues)===settingsBaseline)loadSettings().then(()=>{if(s.apply.kind==='settings_revert')text('settings-result',t('settings_undone'));});else text('settings-result',t('settings_changed'));}}
 if(s.apply?.state==='failed'||s.apply?.state==='reverted')text('settings-result',t('settings_'+s.apply.state)+(typeof s.apply.reason==='string'?' '+s.apply.reason:s.apply.error?.message?' '+s.apply.error.message:''));
 currentScene=s.obs.scene_uuid;
 if(pendingScene && (currentScene===pendingScene || !s.obs.connected)){clearTimeout(sceneTimer);pendingScene='';}
 if(s.obs.connected){text('error','');if(!wasConnected || (!sceneList.some(x=>x.uuid===currentScene) && Date.now()-scenesLoadedAt>=1000))loadScenes();}else{sceneList=[];}wasConnected=s.obs.connected;drawScenes();
  if(s.apply?.state === 'applying' && s.apply?.kind === 'restart') { if(!restarting) since=Date.now(); restarting=true; connection(); } else { restarting=false; text('connection',''); }
  const summary=[];
  el('cameras').replaceChildren(...(s.cameras || []).map(c => {
   cameraNames.set(c.id,c.name);const p=document.createElement('div');p.className='cam';p.dataset.state=c.state;
   if(states.has(c.id)&&states.get(c.id)!==c.state)p.classList.add('flash');states.set(c.id,c.state);
   const add=(tag,value,cls,parent=p)=>{const e=document.createElement(tag);e.textContent=value;e.className=cls;parent.append(e);return e;};
   const name=add('span',c.name,'cam-name');add('span',c.state,'state',name);
   const rate=add('span','','cam-read');
   add('b',Math.round(c.kbps).toLocaleString(),'rate-value',rate);rate.append(document.createTextNode(' '));add('small','kbps','rate-unit',rate);
   const now=Date.now(), samples=(readings.get(c.id)||[]).filter(x=>now-x[0]<=60000);samples.push([now,c.kbps]);readings.set(c.id,samples);
   const max=Math.max(1,c.recover_kbps*2.5,...samples.map(x=>x[1]));
   const strip=add('div','','strip');strip.setAttribute('aria-hidden','true');add('div','','fill',strip).style.width=Math.min(100,c.kbps/max*100)+'%';
   for(const value of [c.fail_kbps,c.recover_kbps]){const tick=add('div','','tick',strip);tick.style.left=Math.min(100,value/max*100)+'%';add('span',String(value),'',tick);}
   const protocol=(c.protocol||'').toUpperCase();
   const separator=lang==='ja'?'、':', ';
   const sub=add('span',protocol+(c.srt?separator+t('rtt')+' '+c.srt.rtt_ms+' ms'+separator+t('loss')+' '+c.srt.loss_rate+'%':''),'cam-sub');
   const link=add('button',t('sender_link'),'btn to-send',sub);link.onclick=()=>{senderCamera=c.id;document.querySelector('[data-page="sender"]').click();};
   const chip=document.createElement('span');chip.className='chip';chip.dataset.state=c.state;chip.textContent=c.name+' '+c.state;summary.push(chip);return p;
  }));
  const o=s.obs, v=o.standby;
  returnScene=v.return_scene||'';
  const waiting=v.owner==='auto'||v.owner==='manual', label=t(v.owner==='auto'?'standby_auto':'standby_manual');
  document.body.classList.toggle('in-standby',waiting);
  if(waiting){const chip=document.createElement('span');chip.className='chip standby-chip';chip.textContent=label;summary.unshift(chip);}
  el('summary').replaceChildren(...summary);
  text('standby-label',label);text('return-button',t('return_to').replace('{scene}',returnScene||t('none')));
  el('return-button').disabled=!returnScene;
  el('manual-button').hidden=v.owner==='manual';el('manual-hint').hidden=v.owner==='manual';
  text('manual-button',t(v.owner==='auto'?'manual_hold':'manual_save'));text('manual-hint',t(v.owner==='auto'?'manual_hold_hint':'manual_save_hint'));
  text('tally',t(o.streaming?'streaming':'not_streaming'));el('tally').classList.toggle('off',!o.streaming);drawScenes();
  const separator=lang==='ja'?'、':', ';
  el('obs').dataset.connected=String(o.connected);
  text('obs',t(o.connected?'obs_connected':'obs_disconnected')+separator+t(o.studio_mode?'studio_on':'studio_off'));
  text('standby',(v.owner==='auto'||v.owner==='manual') ? t(v.owner==='auto'?'standby_auto':'standby_manual')+separator+t('return_scene_is').replace('{scene}',v.return_scene || t('none')) : t('standby_inactive'));
  text('mediamtx',t('mediamtx_'+(s.mediamtx||'stopped')) || s.mediamtx);
  const notices=[];
  if(s.last_error?.message) notices.push(s.last_error.message);
  notices.push(...(o.warnings || []),...(s.warnings || []));
  for(let i=notices.length-1;i>=0;i--)if(notices.indexOf(notices[i])!==i)notices.splice(i,1); // 同じ文は1回だけ
  const settingsWarnings=[...(o.warnings||[]),...(s.warnings||[])];if(settingsWarnings.length){text('settings-result',settingsWarnings.join(' '));text('sender-notice',settingsWarnings.join(' '));}
  el('warnings').replaceChildren(...notices.map(message=>{const item=document.createElement('p');item.className='warn';item.textContent=message;return item;}));
 }
 request('status').then(render).catch(e=>text('error',e.message));
 let logLevel='INFO'; let logLines=[]; let stickToBottom=true;
 function drawLogs(){ const box=el('log-lines'); box.textContent=logLines.map(x=>new Date(x.t).toLocaleTimeString()+' '+x.level+' '+x.text).join('\n'); if(stickToBottom) box.scrollTop=box.scrollHeight; }
 async function loadLogs(){ try { logLines=await request('logs?lines=500&level='+logLevel); drawLogs(); } catch(e){ text('error',e.message); } }
 el('log-level').addEventListener('change',()=>{logLevel=el('log-level').value;loadLogs();}); el('log-lines').addEventListener('scroll',()=>{const b=el('log-lines');stickToBottom=b.scrollHeight-b.scrollTop-b.clientHeight<8;});
 const events = new EventSource('/api/events?topics=status,log&lang='+lang);
 events.addEventListener('status',e=>render(JSON.parse(e.data)));
 events.addEventListener('log',e=>{const row=JSON.parse(e.data);const rank={INFO:0,WARN:1,ERROR:2};if((rank[row.level]??0)>=(rank[logLevel]??0)){logLines.push(row);if(logLines.length>2000)logLines=logLines.slice(-2000);if(!el('log-panel').hidden)drawLogs();}});
 events.onerror = connection;
 setInterval(()=>{if(restarting)connection();},1000);
}

// 安全でない文脈でも、選択範囲を持つ一時欄でコピーを試す。
async function copySenderValue(value){
 if(navigator.clipboard?.writeText){try{await navigator.clipboard.writeText(value);return true;}catch(_){}}
 const area=document.createElement('textarea');area.value=value;area.readOnly=true;area.contentEditable='true';area.style.position='fixed';area.style.top='0';area.style.left='0';area.style.opacity='0';document.body.append(area);
 const active=document.activeElement;let copied=false;try{area.focus();area.select();area.setSelectionRange(0,value.length);copied=document.execCommand('copy');}catch(_){}finally{area.remove();active?.focus();}return copied;
}
