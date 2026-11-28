// The HTML rules of the preview. A failed read (the network, or an answer that is not ok) is tried again, as the source page does.
export async function loadRules(fetcher=fetch,wait=ms=>new Promise(resolve=>setTimeout(resolve,ms))){
 for(;;){
  try{
   const r=await fetcher('/source/assets/html_rules.json');
   if(!r.ok)throw Error('HTML rules unavailable');
   return await r.json();
  }catch(_){await wait(1000);}
 }
}
