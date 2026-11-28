export const groups={stream:['stream'],obs:['obs','on_stream_start'],admin:['admin','mediamtx'],other:['gui','log','language']};
export const dirty=s=>!!s.data&&JSON.stringify(s.values)!==s.baseline;
export function fieldGroup(path,locations){const p=path.replace(/\[(?:\d+|cam[1-8])\]/g,'[]');if(locations?.[p])return locations[p].group;if(p.startsWith('cameras'))return 'camera';if(p.startsWith('switch.')||['mediamtx.rtmp_enabled','mediamtx.srt_enabled','stream.app','stream.publish_host','stream.publish_rtmp_port','stream.publish_srt_port','stream.publish_user','stream.publish_pass'].includes(p))return 'common';if(p.startsWith('on_stream_start.')||p.startsWith('obs.'))return 'obs';return Object.keys(groups).find(g=>groups[g].includes(p.split('.')[0]))||'admin';}

// Removed IDs last for this draft only; a new saved baseline permits reuse.
export function nextCameraID(saved=[],draft=[],removed=[]){
 if(draft.length>=8)return null;
 const used=new Set([...saved.map(c=>c.id),...draft.map(c=>c.id),...removed]);
 for(let n=1;n<=8;n++)if(!used.has('cam'+n))return 'cam'+n;
 return null;
}
export function cameraAddReason(saved=[],draft=[],removed=[]){
 return draft.length>=8?'settings_camera_limit':nextCameraID(saved,draft,removed)===null?'settings_camera_no_id':'';
}
export {cameraHeading} from './camera-name.js';
// The label (suffix '') or the help (suffix '_help') of a setting. Most settings are named by the last part of the path (setting_<key>), but a few
// keys mean different things in different places, so these paths have a text of their own: setting_<path> (obs.port is the port of the OBS
// WebSocket, admin.port is the port that the admin page opens on).
export const specificSettings=['obs.port','obs.password','admin.port','admin.password','on_stream_start.enabled'];
export function settingText(t,path,key,suffix=''){
 return t('setting_'+(specificSettings.includes(path)?path:key)+suffix);
}
import {cameraHeading} from './camera-name.js';
import {confirmSpec} from './confirm-dialog.js';
export function focusSettingError(errors,doc){
 const path=Object.keys(errors||{})[0];
 if(!path)return;
 const input=doc.getElementById('setting-'+path);
 // Open all enclosing details before focusing an override field.
 for(let parent=input?.closest('details');parent;parent=parent.parentElement?.closest('details'))parent.open=true;
 input?.focus();
 input?.scrollIntoView({block:'center'});
}
// The camera ids that a save would remove (an added or edited camera is not one).
export const removedCameraIDs=preview=>(preview.fields||[]).filter(f=>f.kind==='removed').map(f=>f.path.match(/^cameras\[(cam[1-8])\]$/)?.[1]).filter(Boolean);
// What GET /api/sources says about the sources and the cameras they use; null (or an unreadable list) is an unknown.
export function cameraUsage(data){
 if(!data||data.code==='sources.unreadable'||!Array.isArray(data.sources))return {error:true,sources:[]};
 return {error:false,sources:data.sources.map(s=>({name:s.name,title:s.title||s.name,cameras:s.cameras||[]}))};
}
// Lines for the removed cameras: the titles of the sources that use their values, and what happens to those.
export function cameraUsageLines(ids,usage,t,saved={}){
 if(!ids.length||!usage)return [];
 if(usage.error)return [t('settings_camera_sources_unreadable')];
 const lines=[];
 for(const id of ids){
  const titles=usage.sources.filter(s=>s.cameras.includes(id)).map(s=>s.title);
  if(titles.length)lines.push(t('settings_camera_used').replace('{camera}',()=>cameraHeading(saved.cameras?.find(c=>c.id===id)||{id},t)).replace('{sources}',()=>titles.join(', ')));
 }
 return lines.length?[...lines,t('settings_camera_used_note')]:lines;
}
// A save that adds or removes a camera restarts everything that receives video.
const cameraListChanged=preview=>(preview.fields||[]).some(f=>['added','removed'].includes(f.kind)&&/^cameras\[(cam[1-8])\]$/.test(f.path));
// A save that touches a camera rewrites the cameras section of config.yaml, and its comments are lost (said here, not on the page).
const cameraFieldChanged=preview=>(preview.fields||[]).some(f=>/^cameras\[/.test(f.path));
// How the change is applied: a save says 「保存すると…」, creating a password again says 「作り直すと…」, a plain restart changes no setting.
const modeKey=(action,preview)=>action==='restart'?'settings_restart_only':action==='regenerate'?(preview.apply==='restart'?'settings_regenerate_restart_mode':'settings_regenerate_immediate'):preview.apply==='restart'?'settings_restart_mode':'settings_immediate';
// The lines under the title of the confirmation window: what happens.
export function confirmationLines(action,payload,preview,streaming,t,saved={},usage=null){
 const lines=[];
 if(action==='regenerate'&&payload.target==='publish'){
  if(payload.camera)lines.push(cameraHeading({id:payload.camera},t));
  // While streaming, the warning of the restart (below) already says that the reception drops: one line, not two.
  if(!(streaming&&preview.apply==='restart'))lines.push(t('publish_reset_warning'));
 }
 lines.push(t(modeKey(action,preview)));
 if(streaming&&preview.apply==='restart')lines.push(t('settings_stream_warning'));
 // While streaming, the warning above already says that the reception breaks.
 if(!streaming&&cameraListChanged(preview))lines.push(t('settings_cameras_reset'));
 for(const f of preview.fields||[]){
  const key=f.path.replace(/\[(?:\d+|cam[1-8])\]/g,'').split('.').pop(),match=f.path.match(/^cameras\[(cam[1-8])\]/);
  const camera=match&&(typeof f.camera_name==='string'?{id:match[1],name:f.camera_name}:((f.kind==='removed'?saved.cameras:payload?.values?.cameras)?.find(c=>c.id===match[1])||saved.cameras?.find(c=>c.id===match[1])||{id:match[1]}));
  // A camera that is added or deleted reads 「カメラ 2（サブカメラ）を削除」; a changed field of a camera reads 「カメラ 2（サブカメラ） — 名前」.
  if(camera&&['added','removed'].includes(f.kind)&&f.path===match[0])lines.push(t('settings_'+f.kind+'_camera').replace('{camera}',()=>cameraHeading(camera,t)));
  else lines.push((camera?cameraHeading(camera,t)+' — ':'')+(f.kind?t('settings_'+f.kind):settingText(t,f.path,key)));
 }
 lines.push(...cameraUsageLines(removedCameraIDs(preview),usage,t,saved));
 if(action==='save'&&cameraFieldChanged(preview))lines.push(t('settings_camera_comments'));
 if(preview.admin_lan_will_close)lines.push(t('settings_lan_close'));
 return lines;
}
// The title of the confirmation window: 「〜しますか？」.
// The camera page says so (「カメラの設定を保存しますか？」, S2-97); page is 'cameras' or 'settings'.
export function confirmationTitle(action,payload,t,page='settings'){
 if(action==='restart')return t('confirm_restart');
 if(action==='regenerate')return t('confirm_regenerate_'+payload.target);
 return t(page==='cameras'?'confirm_save_cameras':'confirm_save');
}
// A red 「はい」 that opens on 「いいえ」 (spec v0.37; chapter 5, \"a restart while streaming is shown strongly in red\"):
// an operation that deletes a camera, and, while streaming, an operation that restarts the app (and so breaks the stream's reception and
// the OBS video): restarting, a save that applies by restarting, and regenerating a password that restarts. It is the condition under which
// the lines warn (settings_stream_warning). Not streaming, saving, restarting and regenerating are blue.
export const restartsApp=(action,preview)=>action==='restart'||preview?.apply==='restart';
export const confirmationDanger=(preview,action='save',streaming=false)=>removedCameraIDs(preview||{}).length>0||(!!streaming&&restartsApp(action,preview));
export function confirmationSpec(action,payload,preview,streaming,t,saved={},usage=null,extraLines=[],page='settings'){
 return confirmSpec(t,{title:confirmationTitle(action,payload,t,page),lines:[...confirmationLines(action,payload,preview,streaming,t,saved,usage),...extraLines],danger:confirmationDanger(preview,action,streaming)});
}

// ---- Deleting a camera leaves its card (A-9): the camera leaves the draft values (so the draft, the unsaved mark and the save work as before),
// and the card stays as 「保存すると削除します」 with 「取り消す」. The removed row is kept to put it back.
const byID=(a,b)=>a.id.localeCompare(b.id);
const isSaved=(s,id)=>!!s.data?.values?.cameras?.some(c=>c.id===id);
export function removeCamera(s,id){
 const row=s.values.cameras.find(c=>c.id===id);
 if(!row)return false;
 s.values.cameras=s.values.cameras.filter(c=>c.id!==id);
 s.removedCameraIDs=[...new Set([...(s.removedCameraIDs||[]),id])];
 if(isSaved(s,id))s.removedRows={...s.removedRows,[id]:structuredClone(row)};
 return true;
}
export function undoCameraRemoval(s,id){
 if(!isSaved(s,id)||s.values.cameras.some(c=>c.id===id))return false;
 const row=structuredClone(s.removedRows?.[id]||s.data.values.cameras.find(c=>c.id===id));
 s.values.cameras=[...s.values.cameras,row].sort(byID);
 s.removedCameraIDs=(s.removedCameraIDs||[]).filter(x=>x!==id);
 const rows={...s.removedRows};delete rows[id];s.removedRows=rows;
 return true;
}
// The cards of the camera page in order of their ids: the cameras of the draft, and the saved cameras that the draft deletes (removing: true).
export function cameraCards(values=[],saved=[],removedIDs=[],rows={}){
 const cards=values.map(row=>({id:row.id,row,removing:false}));
 for(const id of new Set(removedIDs)){
  const base=saved.find(c=>c.id===id);
  if(base&&!values.some(c=>c.id===id))cards.push({id,row:rows[id]||base,removing:true});
 }
 return cards.sort(byID);
}

// The range of a number field (A-16): the same bounds that the server checks, written on the input as min and max.
// Ports are 1 to 65535 (the port that is sent to the cameras may be 0, which means the same as the MediaMTX one); the seconds are 1 and more;
// kbps start at 0; the days of the log are kept at least 1 day.
export function numberBounds(path,key){
 if(key==='port'||key.endsWith('_port'))return {min:key.startsWith('publish_')?0:1,max:65535};
 if(path==='log.keep_days')return {min:1,max:3650};
 if(key.endsWith('_sec'))return {min:1,max:86400};
 if(key.endsWith('_kbps'))return {min:0,max:1000000};
 return {};
}
