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
document.querySelectorAll('[data-language]').forEach(b => b.addEventListener('click', () => { const next=b.dataset.language; try { localStorage.setItem('language', next); } catch (_) {} location.reload(); }));
const el = id => document.getElementById(id);
const text = (id, value) => { el(id).textContent = value; };
document.documentElement.lang = lang; document.title = t('title');
document.querySelectorAll('[data-ui]').forEach(e => e.textContent = t(e.dataset.ui));
async function request(path, body) {
 const r = await fetch('/api/' + path + (path.includes('?') ? '&' : '?') + 'lang=' + lang, {method: body === undefined ? 'GET' : 'POST', headers: {'Content-Type':'application/json','X-IRL-Live-Keeper':'1'}, body: body === undefined ? undefined : JSON.stringify(body)});
 if(r.status === 401 && !boot.login) location.replace('/?login');
 const data = await r.json(); if (!r.ok) throw Error(data.error || t('failed')); return data;
}
if (boot.login) el('login').hidden = false; el('admin').hidden = boot.login;
el('login')?.addEventListener('submit', async e => { e.preventDefault(); try { await request('login', {password:el('password').value}); location.replace('/'); } catch(e) { text('error',e.message); } });
if (!boot.login) {
 if (matchMedia('(max-width:700px)').matches) el('menu').open = false;
 document.querySelectorAll('[data-page]').forEach(b => b.onclick = () => { const isLogs=b.dataset.page==='logs'; el('status').hidden = b.dataset.page !== 'status'; el('log-panel').hidden=!isLogs; el('soon').hidden = b.dataset.page==='status'||isLogs; if(isLogs) loadLogs(); });
 document.querySelectorAll('[data-command]').forEach(b => b.onclick = async () => {
  const path = b.dataset.command;
  if(path === 'stream/stop' && !confirm(t('confirm_stop'))) return;
  if(path === 'cameras/show-all' && !confirm(t('confirm_show'))) return;
  b.disabled = true; try { await request(path, {}); text('error',''); } catch(e) { text('error',e.message); } finally { b.disabled = false; }
 });
 let restarting = false, since = 0;
 function connection() { text('connection', restarting ? t('restarting') + (Date.now()-since >= 30000 ? ' ' + t('restart_help') : '') : t('disconnected')); }
 let sceneList=[], currentScene='', pendingScene='', sceneTimer;
 function drawScenes(){el('scenes').replaceChildren(...sceneList.map(s=>{const b=document.createElement('button');const current=s.uuid===currentScene;b.textContent=s.name+(current?' · '+t('current_scene'):'');b.setAttribute('aria-pressed',String(current));b.disabled=!!pendingScene;b.onclick=async()=>{pendingScene=s.uuid;drawScenes();sceneTimer=setTimeout(()=>{pendingScene='';drawScenes();text('error',t('failed'));},10000);try{await request('obs/scene',{scene_uuid:s.uuid});}catch(e){clearTimeout(sceneTimer);pendingScene='';drawScenes();text('error',e.message);}};return b;}));}
 let loadingScenes=false, scenesLoadedAt=0;
 async function loadScenes(){if(loadingScenes)return;loadingScenes=true;scenesLoadedAt=Date.now();try{sceneList=await request('obs/scenes');drawScenes();}catch(e){text('error',e.message);}finally{loadingScenes=false;}}
 loadScenes();
 setInterval(()=>{if(wasConnected)loadScenes();},10000);
 let wasConnected=false;
 function render(s) {
 currentScene=s.obs.scene_uuid;
 if(pendingScene && (currentScene===pendingScene || !s.obs.connected)){clearTimeout(sceneTimer);pendingScene='';}
 if(s.obs.connected && (!wasConnected || (!sceneList.some(x=>x.uuid===currentScene) && Date.now()-scenesLoadedAt>=1000)))loadScenes();wasConnected=s.obs.connected;drawScenes();
  if(s.apply?.state === 'applying' && s.apply?.kind === 'restart') { if(!restarting) since=Date.now(); restarting=true; connection(); } else { restarting=false; text('connection',''); }
  el('cameras').replaceChildren(...(s.cameras || []).map(c => { const p=document.createElement('p'); p.textContent=c.name+' : '+c.state+' · '+c.kbps+' kbps'+(c.protocol === 'srt' && c.srt ? ' · RTT '+c.srt.rtt_ms+' ms · '+c.srt.loss_rate+' %' : ''); return p; }));
  const o=s.obs, v=o.standby;
  text('obs',[t(o.connected?'connected':'offline'),t(o.streaming?'streaming':'not_streaming'),t('scene')+': '+o.scene,t('studio')+': '+t(o.studio_mode?'on':'off')].join(' · '));
  text('standby',t(v.owner || 'none')+' · '+t('return_scene')+': '+(v.return_scene || t('none')));
  text('mediamtx',t(s.mediamtx) || s.mediamtx);
  text('last-error',s.last_error?.message || '');
  el('warnings').replaceChildren(...(o.warnings || []).map(w=>{const li=document.createElement('li');li.textContent=w;return li;}));
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
