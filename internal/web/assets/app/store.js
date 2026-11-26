export function createStore(initial) {
 let state=structuredClone(initial), queued=false;
 const listeners=new Set();
 const freeze=x=>{if(x&&typeof x==='object'){Object.freeze(x);Object.values(x).forEach(freeze);}return x;};
 freeze(state);
 return {get:()=>state, update(fn){const next=structuredClone(state);fn(next);state=freeze(next);if(!queued){queued=true;queueMicrotask(()=>{queued=false;for(const f of listeners){try{f(state);}catch(e){console.error(e);}}});}},subscribe(fn){listeners.add(fn);return ()=>listeners.delete(fn);}};
}
