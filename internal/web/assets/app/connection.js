// DOM-free helpers for the connection, the notices and the reads that follow a restart (spec v0.32, chapter 5).
// A notice carries noticeNetwork when it came from a failed request, so that coming back online can remove it.
export const isOnline=s=>!s.disconnected&&!s.restarting;
// True when the state moved from unreachable (stopped or restarting) to reachable.
export const reconnected=(before,after)=>!isOnline(before)&&isOnline(after);
export function setNotice(s,e){s.notice=e.message;s.noticeNetwork=!!e.network;}
export function clearNotice(s){s.notice='';s.noticeNetwork=false;}
export function clearNetworkNotice(s){if(s.noticeNetwork)clearNotice(s);}
export const applyKey=status=>JSON.stringify([status.generation,status.apply]);
// After the application applied (or reverted) a save, the settings and the destinations are read again.
export const reloadAfterApply=(previous,key,status)=>previous!==undefined&&previous!==key&&['applied','reverted'].includes(status.apply?.state);
// Reading the destinations: not while the application restarts (there is nothing to read, and no failure to show).
export function createSenderLoader({request,get,update,notice}){
 return async function loadSender(){
  if(get().restarting)return false;
  try{
   const sender=await request('sender');
   const host=get().host;
   update(s=>{s.sender=sender;s.host=sender.addresses.some(a=>a.address===host)?host:sender.default;});
   return true;
  }catch(e){notice(e);return false;}
 };
}
