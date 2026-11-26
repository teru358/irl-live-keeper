import {createAPI} from './app/api.js';
import {createI18n} from './app/i18n.js';
'use strict';
const boot = JSON.parse(document.querySelector('#bootstrap').textContent);
const {lang,t,setLanguage}=createI18n(boot);
document.querySelectorAll('[data-language]').forEach(b => { b.setAttribute('aria-pressed',String(b.dataset.language===lang)); b.addEventListener('click', () => { setLanguage(b.dataset.language); }); });
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
const request=createAPI({lang,t,login:boot.login});
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
 document.querySelectorAll('[data-page]').forEach(b => b.onclick = () => { document.body.dataset.page=b.dataset.page;el('sheet').classList.remove('open');menuButton.setAttribute('aria-expanded','false');document.querySelectorAll('[data-page]').forEach(x=>{if(x.dataset.page===b.dataset.page)x.setAttribute('aria-current','page');else x.removeAttribute('aria-current');});const isLogs=b.dataset.page==='logs'; el('settings-panel').hidden=b.dataset.page!=='settings'; if(b.dataset.page==='settings'&&!settingsData)loadSettings(); el('status').hidden = b.dataset.page !== 'status'; el('log-panel').hidden=!isLogs; el('sender-panel').hidden=b.dataset.page!=='sender'; el('soon').hidden = b.dataset.page==='status'||isLogs||b.dataset.page==='sender'||b.dataset.page==='settings'; window.dispatchEvent(new CustomEvent('keeper-page',{detail:b.dataset.page})); });
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
 function settingsLock(on){settingsBusy=on;for(const id of ['settings-save','settings-revert','settings-restart','settings-accept','settings-reload'])el(id).disabled=on;window.dispatchEvent(new CustomEvent('keeper-settings-lock',{detail:on}));}
 async function previewSettings(action,payload){if(settingsBusy)return;settingsLock(true);try{if(!settingsData)await loadSettings();if(!settingsData)return;settingsPayload=payload||{values:structuredClone(settingsValues),hash:settingsData.hash};settingsAction=action;let preview;
 if(action==='restart')preview={apply:'restart'};else if(action==='regenerate')preview={apply:settingsPayload.target==='admin'?'immediate':'restart',obs_url_changes:settingsPayload.target==='read'};else preview=await request('settings/preview',action==='revert'?{revert:true}:settingsPayload);
 const box=el('settings-preview');box.replaceChildren();if(action==='regenerate')box.append(settingNode('p',t('settings_regenerate_'+settingsPayload.target)));box.classList.toggle('danger',settingsStreaming);box.append(settingNode('p',t(preview.apply==='restart'?'settings_restart_mode':'settings_immediate')));if(settingsStreaming&&preview.apply==='restart')box.append(settingNode('p',t('settings_stream_warning')));for(const f of preview.fields||[]){const key=f.path.replace(/\[\d+\]/g,'').split('.').pop();const camera=f.path.match(/^cameras\[(\d+)\]\.(.+)$/);const fieldLabel=t('setting_'+key);const label=camera?t('setting_cameras')+' '+(Number(camera[1])+1)+' — '+fieldLabel:fieldLabel;box.append(settingNode('p',label));}if(preview.admin_lan_will_close)box.append(settingNode('p',t('settings_lan_close')));for(const f of preview.fields||[])if(f.local_only&&!settingsData.local)box.append(settingNode('p',t('settings_local')));
 el('settings-update-label').hidden=!preview.obs_url_changes;el('settings-update').checked=false;el('settings-confirm').showModal();}catch(e){settingsError(e);}finally{settingsLock(false);}}
 el('settings-save').onclick=()=>previewSettings('save');el('settings-reload').onclick=async()=>{await loadSettings();if(settingsData)text('settings-result',t('settings_reloaded'));};el('settings-revert').onclick=()=>previewSettings('revert');el('settings-restart').onclick=()=>previewSettings('restart');el('settings-cancel').onclick=()=>el('settings-confirm').close();
 async function regenerate(target){if(settingsData&&JSON.stringify(settingsValues)!==settingsBaseline){text('settings-result',t('settings_changed'));return;}await loadSettings();if(settingsData)previewSettings('regenerate',{hash:settingsData.hash,target});}
 el('settings-accept').onclick=async()=>{if(settingsBusy)return;settingsLock(true);try{const action=settingsAction;const payload={...settingsPayload,update_obs_input:el('settings-update').checked};const submitted=JSON.stringify(settingsValues);const result=await request(action==='restart'?'restart':action==='save'?'settings':'settings/'+action,action==='restart'?{}:action==='revert'?{update_obs_input:payload.update_obs_input}:payload,action==='save'?'PUT':'POST');settingsBaseline=submitted;settingsRefreshPending=true;el('settings-confirm').close();text('error','');text('settings-result',t(action==='revert'?'settings_undone':'settings_saved'));if(!el('settings-update-label').hidden&&!payload.update_obs_input)text('settings-result',t('settings_pending_urls'));if(result.apply==='restart'||action==='restart'){window.dispatchEvent(new Event('keeper-restarting'));if(result.admin_url)setTimeout(()=>location.assign(result.admin_url),1500);}else{await loadSettings();text('settings-result',t(action==='revert'?'settings_undone':'settings_applied'));window.dispatchEvent(new CustomEvent('keeper-page',{detail:'sender'}));}}catch(e){el('settings-confirm').close();settingsError(e);}finally{settingsLock(false);}};

 window.addEventListener('keeper-status',event=>{const s=event.detail;settingsStreaming=!!s.obs.streaming;settingsOBS=!!s.obs.connected;
 const applyKey=JSON.stringify([s.generation,s.apply]);if(settingsApplyKey!==applyKey){const previous=settingsApplyKey;settingsApplyKey=applyKey;if(settingsData&&(previous!==undefined||settingsRefreshPending)&&['applied','reverted'].includes(s.apply?.state)){settingsRefreshPending=false;if(JSON.stringify(settingsValues)===settingsBaseline)loadSettings().then(()=>{if(s.apply.kind==='settings_revert')text('settings-result',t('settings_undone'));else if(s.apply.state==='applied')text('settings-result',t('settings_applied'));});else text('settings-result',t('settings_changed'));}}
 if(s.apply?.state==='failed'||s.apply?.state==='reverted')text('settings-result',t('settings_'+s.apply.state)+(typeof s.apply.reason==='string'?' '+s.apply.reason:s.apply.error?.message?' '+s.apply.error.message:''));
 });
 window.addEventListener('keeper-regenerate',e=>regenerate(e.detail));
}
