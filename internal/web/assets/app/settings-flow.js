// DOM-free steps of the settings page, so that Node can run them with fakes (spec chapters 5 and 7, v0.32).
import {fieldGroup,dirty} from './settings-state.js';
import {setNotice,clearNotice} from './connection.js';
const generic=path=>path.replace(/\[(?:\d+|cam[1-8])\]/g,'[]');
// A failed request. Writes the notice and the field errors into the draft state s and returns the page that
// holds the first wrong field when that is another page, otherwise ''. With no field errors (409, 403,
// 500, a failed connection) it only shows the notice.
export function failureState(s,e,t,page){
 setNotice(s,{message:t('settings_not_saved')+' '+e.message,network:e.network});
 s.errors=e.fields||{};
 const first=Object.keys(s.errors)[0];
 if(!first)return '';
 s.group=fieldGroup(first,s.data?.field_locations);
 const target=s.data?.field_locations?.[generic(first)]?.page||page;
 return target===page?'':target;
}
// "Save" with nothing changed does not open the confirmation window.
export const nothingToSave=(action,s)=>action==='save'&&!!s.data&&!dirty(s);
export function noChangesNotice(s,t){clearNotice(s);s.notice=t('settings_no_changes');}
// The saved result: restarting to apply means no read now (the application is going away). The settings and
// the destinations are read again when the status says applied or reverted. Immediate apply reads at once.
export async function finishAccept({action,result,values,preview,updateOBS,t,update,dispatch,load,closeDialog,page,go=()=>{}}){
 update(x=>{x.baseline=JSON.stringify(values);clearNotice(x);x.notice=t('settings_saved');if(preview.obs_url_changes&&!updateOBS)x.notice=t('settings_pending_urls');});
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
