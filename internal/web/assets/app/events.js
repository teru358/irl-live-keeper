export function applyEvent(store,type,data){store.update(s=>{if(type==='status'){s.status=data;s.statusVersion=(s.statusVersion||0)+1;s.disconnected=false;const now=Date.now();for(const c of data.cameras||[]){const old=s.readings[c.id]||[];s.readings[c.id]=[...old.filter(x=>now-x[0]<=60000),[now,c.kbps]];s.flashes[c.id]=s.cameraStates[c.id]!==undefined&&s.cameraStates[c.id]!==c.state;s.cameraStates[c.id]=c.state;}if(s.pendingScene&&(data.obs.scene_uuid===s.pendingScene||!data.obs.connected))s.pendingScene='';}});}
// A stream of server events that survives the process being started again. EventSource closes for good on an answer that is
// not 200 (after a start the session is gone and /api/events says 401), so when it is closed the page asks with an ordinary
// request (a 401 moves it to the login) and opens the stream again once that works. A stream that is still trying is left alone.
export function openStream(create,{probe,delay=3000,onError=()=>{},timers={}}={}){
 const setTimer=timers.set||setTimeout,clearTimer=timers.clear||clearTimeout;
 let source,timer=null,closed=false;
 const connect=()=>{
  const mine=create();source=mine;
  mine.onerror=()=>{onError();if(source===mine&&mine.readyState===2)retry();};
 };
 const retry=()=>{
  if(closed||timer||!probe)return;
  timer=setTimer(async()=>{
   timer=null;if(closed)return;
   try{await probe();}catch(e){if(e?.status!==401)retry();return;}
   if(!closed)connect();
  },delay);
 };
 connect();
 return ()=>{closed=true;clearTimer(timer);source.close();};
}
export function connectEvents(store,lang,Source=EventSource,options={}){
 return openStream(()=>{
  const source=new Source('/api/events?topics=status&lang='+lang);
  source.addEventListener('status',e=>{try{applyEvent(store,'status',JSON.parse(e.data));}catch(e){console.error(e);}});
  return source;
 },{...options,onError:()=>store.update(s=>{s.disconnected=true;})});
}
