const equal=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
export const pageDirty=(values,baseline)=>!equal(values,baseline);
export function projectPage(values,page,locations){
 function walk(value,path){
  if(Array.isArray(value))return Object.entries(locations).some(([p,l])=>p.startsWith(path+'[].')&&l.page===page)?structuredClone(value):undefined;
  if(value&&typeof value==='object'){const out={};for(const [k,v] of Object.entries(value)){const projected=walk(v,path?path+'.'+k:k);if(projected!==undefined)out[k]=projected;}return Object.keys(out).length?out:undefined;}
  return locations[path]?.page===page?value:undefined;
 }
 return walk(values,'')||{};
}
export function rebaseDraft(base,draft,next){
 const conflicts=[];
 function merge(a,b,c,path){
  if(equal(a,b))return structuredClone(c);
  if(equal(a,c))return structuredClone(b);
  if(equal(b,c))return structuredClone(c);
  if(path==='cameras'&&[a,b,c].every(Array.isArray)){
   const ids=new Set([...a,...b,...c].map(x=>x.id));const rows=[];
   for(const id of [...ids].sort()){const row=merge(a.find(x=>x.id===id),b.find(x=>x.id===id),c.find(x=>x.id===id),`cameras[${id}]`);if(row!==undefined)rows.push(row);}return rows;
  }
  if([a,b,c].every(x=>x&&typeof x==='object'&&!Array.isArray(x))){const out={};for(const k of new Set([...Object.keys(a),...Object.keys(b),...Object.keys(c)])){const v=merge(a[k],b[k],c[k],path?path+'.'+k:k);if(v!==undefined)out[k]=v;}return out;}
  conflicts.push(path);return structuredClone(c);
 }
 return {values:merge(base,draft,next,''),conflicts};
}
