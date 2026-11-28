import {styleStart} from './template-lexer.js';
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
const groupRules=new Set(['media','supports','container','layer','scope','document','-moz-document']);
// Where the end of a piece of CSS is (spec chapter 14, v0.32).
// 'declaration': inside the {…} of a rule, outside comments and strings.
// 'rule': outside rules (before the first '{', between rules, directly inside a grouping at-rule such as @media).
// 'literal': inside a comment or a string, or directly inside @keyframes. Only a url("…") goes there.
export function cssContext(css){
 const stack=[];let head='',quote=null,comment=false,paren=0;
 for(let i=0;i<css.length;i++){
  const c=css[i];
  if(comment){if(c==='*'&&css[i+1]==='/'){comment=false;i++;}continue;}
  if(quote){if(c==='\\')i++;else if(c===quote)quote=null;continue;}
  if(c==='/'&&css[i+1]==='*'){comment=true;i++;continue;}
  if(c==='"'||c==="'"){quote=c;continue;}
  if(c==='('){paren++;head+=c;continue;}
  if(c===')'){paren=Math.max(0,paren-1);head+=c;continue;}
  if(paren>0){head+=c;continue;}
  if(c==='{'){
   const at=/^@([A-Za-z0-9_-]+)/.exec(head.trim());
   const name=at?.[1].toLowerCase();
   stack.push(name&&/keyframes$/.test(name)?'keyframes':name&&groupRules.has(name)?'group':'declaration');
   head='';
  }else if(c==='}'){stack.pop();head='';}
  else if(c===';')head='';
  else head+=c;
 }
 if(comment||quote)return 'literal';
 const top=stack[stack.length-1];
 return top===undefined||top==='group'?'rule':top==='keyframes'?'literal':'declaration';
}
// The text that goes in: a rule that shows the image where the cursor is outside rules, otherwise only the url.
export function assetInsertText(path,context){
 const url='url("'+path+'")';
 return context==='rule'?'.image { background: '+url+' no-repeat; }':url;
}
export function bodyAssetInsertion(doc,selection,hasCursor,name){
 const from=hasCursor?selection.from:doc.length,to=hasCursor?selection.to:doc.length;
 const path='/source/files/'+name.split('/').map(encodeURIComponent).join('/');
 const prefix=doc.slice(0,from),start=styleStart(prefix);
 return {from,to,insert:start<0?'<img src="'+path+'">':assetInsertText(path,cssContext(prefix.slice(start)))};
}
export function diagnosticRange(text,column){
 const from=Array.from(text).slice(0,Math.max(0,column-1)).join('').length;
 return {from,to:Math.min(text.length,from+(text.codePointAt(from)>0xffff?2:1))};
}
