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
import {cameraHeading} from './camera-name.js';
export function focusSettingError(errors,doc){
 const path=Object.keys(errors||{})[0];
 if(!path)return;
 const input=doc.getElementById('setting-'+path);
 // Open all enclosing details before focusing an override field.
 for(let parent=input?.closest('details');parent;parent=parent.parentElement?.closest('details'))parent.open=true;
 input?.focus();
 input?.scrollIntoView({block:'center'});
}
export function confirmationLines(action,payload,preview,streaming,t,saved={}){
 const lines=[];
 if(action==='regenerate')lines.push(t('settings_regenerate_'+payload.target));
 if(action==='regenerate'&&payload.target==='publish'){if(payload.camera)lines.push(cameraHeading({id:payload.camera},t));lines.push(t('publish_reset_warning'));}
 lines.push(t(preview.apply==='restart'?'settings_restart_mode':'settings_immediate'));
 if(streaming&&preview.apply==='restart')lines.push(t('settings_stream_warning'));
 for(const f of preview.fields||[]){
  const key=f.path.replace(/\[(?:\d+|cam[1-8])\]/g,'').split('.').pop(),match=f.path.match(/^cameras\[(cam[1-8])\]/);
  const camera=match&&(typeof f.camera_name==='string'?{id:match[1],name:f.camera_name}:((f.kind==='removed'?saved.cameras:payload?.values?.cameras)?.find(c=>c.id===match[1])||saved.cameras?.find(c=>c.id===match[1])||{id:match[1]}));
  lines.push((camera?cameraHeading(camera,t)+' — ':'')+t(f.kind?'settings_'+f.kind:'setting_'+key));
 }
 if(action==='revert')lines.push(t('revert_credentials'));
 if(preview.admin_lan_will_close)lines.push(t('settings_lan_close'));
 return lines;
}
