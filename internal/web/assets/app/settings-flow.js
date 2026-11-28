// DOM-free steps of the settings page, so that Node can run them with fakes (spec chapters 5 and 7, v0.35).
import {fieldGroup,dirty} from './settings-state.js';
import {projectPage,rebaseDraft} from './settings-pages.js';
import {setNotice,clearNotice} from './connection.js';
const generic=path=>path.replace(/\[(?:\d+|cam[1-8])\]/g,'[]');
// A failed request. Writes the notice and the field errors into the draft state s and returns the page that
// holds the first wrong field when that is another page, otherwise ''. With no field errors (409, 403,
// 500, a failed connection) it only shows the notice. A failure of a read (saving=false) is not a failure to save.
export function failureState(s,e,t,page,saving=true){
 setNotice(s,{message:(saving?t('settings_not_saved')+' ':'')+e.message,network:e.network});
 s.errors=e.fields||{};
 const first=Object.keys(s.errors)[0];
 if(!first)return '';
 s.group=fieldGroup(first,s.data?.field_locations);
 const target=s.data?.field_locations?.[generic(first)]?.page||page;
 return target===page?'':target;
}
// The page that holds the wrong field gets the errors and the notice of the page that sent the request. When that page
// has not read its settings yet, the read that its opening starts keeps them (carryErrors) once.
export function receiveErrors(s,{errors,message,network}){
 setNotice(s,{message,network});
 s.errors=errors||{};
 const first=Object.keys(s.errors)[0];
 if(first)s.group=fieldGroup(first,s.data?.field_locations);
 s.carryErrors=!s.data;
}
// The settings that were read, put into the page state. A read in the background puts the draft written meanwhile on the
// new saved values (rebaseDraft); a read the user asked for replaces the draft.
export function applyLoaded(s,{data,scenes,page,background,t,normalize}){
 const fresh=normalize(projectPage(data.values,page,data.field_locations));
 const rebased=background&&s.data?rebaseDraft(JSON.parse(s.baseline),s.values,fresh):{values:fresh,conflicts:[]};
 s.data=data;s.values=rebased.values;s.baseline=JSON.stringify(fresh);
 if(rebased.conflicts.length)s.notice=t('settings_conflicts')+' '+rebased.conflicts.join(', ');
 s.scenes=scenes;
 if(!s.carryErrors)s.errors={};
 s.carryErrors=false;
 if(!background){s.removedCameraIDs=[];s.applyNotice='';}
}
// A status event: the states of the cameras, whether the stream runs, whether OBS is connected (the scene choice needs
// it), and the notice of a failed or reverted apply.
export function applyStatus(s,status,t){
 s.cameraStatus=Object.fromEntries((status.cameras||[]).map(c=>[c.id,c]));
 s.streaming=!!status.obs.streaming;
 s.obs=!!status.obs.connected;
 s.applyNotice=['failed','reverted'].includes(status.apply?.state)?t('settings_'+status.apply.state)+' '+(status.apply.reason||status.apply.error?.message||''):'';
}
// OBS has just become connected and the page holds its settings: the scenes are read now (a page opened later reads them in its load).
export const sceneReadNeeded=(wasConnected,s)=>!wasConnected&&!!s.obs&&!!s.data;
// "Save" with nothing changed does not open the confirmation window.
export const nothingToSave=(action,s)=>action==='save'&&!!s.data&&!dirty(s);
export function noChangesNotice(s,t){clearNotice(s);s.notice=t('settings_no_changes');}
// "Restart" with a draft is refused, as the regenerating of a password is: the restart would carry the draft away.
export const restartBlocked=(action,s)=>action==='restart'&&!!s.data&&dirty(s);
export function restartDirtyNotice(s,t){clearNotice(s);s.notice=t('settings_restart_dirty');}
// "Reload" with a changed draft asks before the draft is thrown away (same form as the sources' "back to list").
export const confirmReload=(s,ask,t)=>!dirty(s)||ask(t('settings_discard'));
// The saved result: restarting to apply means no read now (the application is going away). The settings and
// the destinations are read again when the status says applied or reverted. Immediate apply reads at once.
// A restart alone changes no saved value, so the baseline stays.
export async function finishAccept({action,result,values,preview,updateOBS,t,update,dispatch,load,closeDialog,page,go=()=>{}}){
 update(x=>{if(action!=='restart')x.baseline=JSON.stringify(values);clearNotice(x);x.notice=t(action==='restart'?'settings_restart_accepted':'settings_saved');if(preview.obs_url_changes&&!updateOBS)x.notice=t('settings_pending_urls');});
 closeDialog();
 if(action==='restart'||result.apply==='restart'){
  dispatch('keeper-restarting');
  if(result.admin_url)go(result.admin_url);
  return 'restart';
 }
 dispatch('keeper-settings-refresh',page);
 await load();
 update(x=>{x.notice=t('settings_applied');});
 return 'applied';
}
