// The stream button of the status page (spec v0.37, decision 10): one button that follows the stream state, with the words of OBS
// (「配信開始」「配信終了」). Starting is blue, ending is red. OBS not connected: not pressable, and the reason is written. No DOM.
import {confirmSpec} from './confirm-dialog.js';

// status is the /api/status data (obs.streaming, obs.connected).
export function streamControl(status,t){
 const streaming=!!status?.obs?.streaming,connected=!!status?.obs?.connected;
 return {
  path:streaming?'stream/stop':'stream/start',
  label:t(streaming?'stop':'start'),
  cls:streaming?'btn stop wide':'btn primary wide',
  disabled:!connected,
  reason:connected?'':t('stream_need_obs'),
 };
}

// The scene that starting the stream switches to (on_stream_start in the settings values), or ''.
export const startScene=settings=>{const v=settings?.values?.on_stream_start;return v?.enabled&&v.scene?v.scene:'';};

// The confirmation of the button: starting is a blue 「はい」; ending is red, and opens with the focus on 「いいえ」.
export function streamConfirm(path,t,scene=''){
 if(path==='stream/stop')return confirmSpec(t,{title:t('stream_stop_title'),lines:[t('stream_stop_note')],danger:true});
 return confirmSpec(t,{title:t('stream_start_title'),lines:[t('stream_start_note'),scene?t('stream_start_scene').replace('{scene}',()=>scene):''],danger:false});
}
