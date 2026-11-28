import {insideStyle} from './template-lexer.js';
export const fileAccept='.png,.jpg,.jpeg,.gif,.webp,.avif,.bmp,.svg';
export const sourceTemplates=[
 {key:'plain',body:'<span>{cam1.kbps} kbps</span>'},
 {key:'outline',body:'<style>.text { color: white; -webkit-text-stroke: 1px black; }</style><span class="text">{cam1.kbps} kbps</span>'},
 {key:'background',body:'<style>.box { background: #000a; color: white; padding: 8px; }</style><div class="box">{cam1.name}</div>'},
 {key:'state',body:'<style>.OK { color: lime; } .LOW { color: orange; } .DOWN { color: red; }</style><span class="{cam1.state}">{cam1.state}</span>'},
];
export function templateBody(index,cameras=[]){
 const first=[...cameras].sort((a,b)=>Number(a.id.slice(3))-Number(b.id.slice(3)))[0]?.id||'cam1';
 return sourceTemplates[index].body.replace(/\bcam1\b/g,first);
}
export function bodyAssetInsertion(doc,selection,hasCursor,name){
 const from=hasCursor?selection.from:doc.length,to=hasCursor?selection.to:doc.length;
 const path='/source/files/'+name.split('/').map(encodeURIComponent).join('/');
 return {from,to,insert:insideStyle(doc.slice(0,from))?'url("'+path+'")':'<img src="'+path+'">'};
}
export function diagnosticRange(text,column){
 const from=Array.from(text).slice(0,Math.max(0,column-1)).join('').length;
 return {from,to:Math.min(text.length,from+(text.codePointAt(from)>0xffff?2:1))};
}
