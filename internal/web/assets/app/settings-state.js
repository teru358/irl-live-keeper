export const groups={camera:['cameras','switch','on_stream_start'],stream:['stream'],obs:['obs'],admin:['admin','mediamtx'],other:['gui','log','language']};
export const dirty=s=>!!s.data&&JSON.stringify(s.values)!==s.baseline;
export function fieldGroup(path){return Object.keys(groups).find(g=>groups[g].includes(path.split(/[.[]/)[0]))||'admin';}

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
export function cameraHeading(camera,t){
 return t('settings_camera')+' '+camera.id.slice(3)+(camera.name?'（'+camera.name+'）':'');
}
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
 lines.push(t(preview.apply==='restart'?'settings_restart_mode':'settings_immediate'));
 if(streaming&&preview.apply==='restart')lines.push(t('settings_stream_warning'));
 for(const f of preview.fields||[]){
  const key=f.path.replace(/\[(?:\d+|cam[1-8])\]/g,'').split('.').pop(),match=f.path.match(/^cameras\[(cam[1-8])\]/);
  const camera=match&&(typeof f.camera_name==='string'?{id:match[1],name:f.camera_name}:((f.kind==='removed'?saved.cameras:payload?.values?.cameras)?.find(c=>c.id===match[1])||saved.cameras?.find(c=>c.id===match[1])||{id:match[1]}));
  lines.push((camera?cameraHeading(camera,t)+' — ':'')+t(f.kind?'settings_'+f.kind:'setting_'+key));
 }
 if(preview.admin_lan_will_close)lines.push(t('settings_lan_close'));
 return lines;
}
