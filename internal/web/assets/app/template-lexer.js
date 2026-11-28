import {checkAssetPath} from './html.js';
let assetRules;
export function setCompletionRules(rules){assetRules=rules;}
// DOM-free, shared by StreamLanguage and Node tests. Offsets are UTF-16.
export const templateState = () => ({tag:false,format:false,html:false,element:'',closing:false,quote:null,raw:false,comment:false,cssComment:false,cssQuote:null});
export function nextTemplateToken(text, pos, state) {
 const rest=text.slice(pos);
 const token=(length,style)=>({end:pos+length,style});
 if(!state.tag){
  if(state.comment){
   const end=rest.indexOf('-->');if(end<0)return token(rest.length,'comment');
   state.comment=false;return token(end+3,'comment');
  }
  if(state.raw){
   if(/^<\/style(?=[\s/>])/i.test(rest)){state.raw=false;state.cssQuote=null;state.cssComment=false;}
   else {
    if(state.cssComment){if(rest.startsWith('*/')){state.cssComment=false;return token(2,'comment');}return token(1,'comment');}
    if(state.cssQuote){if(rest[0]===state.cssQuote)state.cssQuote=null;return token(1,'string');}
    if(rest.startsWith('/*')){state.cssComment=true;return token(2,'comment');}
    if(rest[0]==='"'||rest[0]==="'"){state.cssQuote=rest[0];return token(1,'string');}
    const word=/^[A-Za-z_-][A-Za-z0-9_-]*/.exec(rest);
    if(word)return token(word[0].length,/^\s*:/.test(rest.slice(word[0].length))?'propertyName':'atom');
    const number=/^[0-9]+(?:\.[0-9]+)?(?:[a-z%]+)?/i.exec(rest);
    return token(number?number[0].length:1,number?'number':/[{}:;]/.test(rest[0])?'punctuation':null);
   }
  }
  if(!state.html&&rest.startsWith('<!--')){state.comment=true;return token(4,'comment');}
  if(!state.html&&/^<\/?[A-Za-z]/.test(rest)){
   state.html=true;state.closing=rest[1]==='/';state.element='';
   return token(state.closing?2:1,'bracket');
  }
  if(state.html&&!state.quote){
   if(!state.element){const m=/^[A-Za-z][A-Za-z0-9-]*/.exec(rest);if(m){state.element=m[0].toLowerCase();return token(m[0].length,'tagName');}}
   if(rest[0]==='>'){state.html=false;state.raw=state.element==='style'&&!state.closing;return token(1,'bracket');}
   if(rest[0]==='"'||rest[0]==="'"){state.quote=rest[0];return token(1,'string');}
   const attr=/^[A-Za-z_:][A-Za-z0-9_:.-]*/.exec(rest);
   return token(attr?attr[0].length:1,attr?'attributeName':null);
  }
  if(state.quote&&rest[0]===state.quote){state.quote=null;return token(1,'string');}

  if(rest.startsWith('{{')||rest.startsWith('}}'))return token(2,null);
  if(rest[0]==='{'){state.tag=true;return token(1,'bracket');}
  return token(1,rest[0]==='}'?'invalid':state.quote?'string':null);
 }
 if(rest[0]==='}'){state.tag=false;state.format=false;return token(1,'bracket');}
 const space=/^[ \t]+/.exec(rest);if(space)return token(space[0].length,null);
 if(rest[0]==='"'){
  let i=1,valid=true,closed=false;
  while(i<rest.length){
   const c=rest[i++];
   if(c==='"'){closed=true;break;}
   if(c==='\n'||c==='\r'){valid=false;break;}
   if(c==='\\'){if(!['"','\\'].includes(rest[i]))valid=false;i+=i<rest.length?1:0;}
  }
  return token(i,valid&&closed?'string':'invalid');
 }
 if(state.format){
  state.format=false;
  const m=/^(?:[A-Za-z_][A-Za-z0-9_]*|[0-9]+)/.exec(rest);
  if(m)return token(m[0].length,/^(?:[0-6]|hms|ms|s|hm)$/.test(m[0])?'atom':'invalid');
 }
 if(rest[0]===':'){state.format=true;return token(1,'punctuation');}
 const name=/^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*/.exec(rest);
 if(name)return token(name[0].length,/^(if|elif|else|end|and|or|not|true|false)$/.test(name[0])?'keyword':'variableName');
 const number=/^-?[0-9]+(?:\.[0-9]+)?/.exec(rest);
 if(number)return token(number[0].length,'number');
 const op=/^(?:<=|>=|==|!=|[<>()])/.exec(rest);
 return token(op?op[0].length:1,op?'operator':'invalid');
}
export function tokenizeTemplate(text) {
 const state=templateState(), tokens=[];
 for(let pos=0;pos<text.length;){const t=nextTemplateToken(text,pos,state);tokens.push({text:text.slice(pos,t.end),style:t.style});pos=t.end;}
 return tokens;
}

// Completion context at the cursor, including an opening brace without a name.
export function templateCompletion(prefix,explicit=false){
 const state=templateState();let start=0,last=null,format=false;
 for(let pos=0;pos<prefix.length;){
  const wasTag=state.tag,t=nextTemplateToken(prefix,pos,state),text=prefix.slice(pos,t.end);
  if(!wasTag&&state.tag){start=t.end;format=false;}
  if(state.tag&&text===':')format=true;
  last={...t,text};pos=t.end;
 }
 if(!state.tag||format||last?.text.startsWith('"'))return null;
 const word=/[A-Za-z_][A-Za-z0-9_.]*$/.exec(prefix.slice(start));
 if(last?.style==='invalid'&&!(word&&word[0].endsWith('.')))return null;
 const bare=prefix.slice(start)==='';
 const condition=/^(?:if|elif)\s+$/.test(prefix.slice(start));
 if(!word&&!bare&&!condition&&!explicit)return null;
 return {from:word?prefix.length-word[0].length:prefix.length};
}

// Where the CSS of the open <style> starts in the prefix (just after its '>'), or -1 when the end of the prefix is not inside a <style>.
export function styleStart(prefix){
 const state=templateState();let start=-1;
 for(let p=0;p<prefix.length;){const wasRaw=state.raw;p=nextTemplateToken(prefix,p,state).end;if(!wasRaw&&state.raw)start=p;}
 return state.raw?start:-1;
}
export function insideStyle(prefix){return styleStart(prefix)>=0;}
export function bodyCompletion(prefix,names,files,explicit=false){
 const match=/\/source\/files\/([^\s"\'<>():;{}]*)$/.exec(prefix);
 if(match)return {from:prefix.length-match[1].length,options:files.filter(f=>f.usable&&(!assetRules||checkAssetPath(f.name,assetRules))).map(f=>({label:f.name,type:'text'})),validFor:assetPrefix};
 const range=templateCompletion(prefix,explicit);
 return range?{...range,options:names.map(label=>({label,type:'variable'})),validFor:/^[A-Za-z0-9_.]*$/}:null;
}

function assetPrefix(text){
 if(!assetRules)return /^[^\s"'<>():;{}%?#\\]*$/.test(text);
 return text.split('/').every(part=>[...part].every(c=>{
 const n=c.codePointAt(0);return c==='_'||c==='-'||c==='.'||assetRules.asset.allowedRanges.some(([a,b])=>a<=n&&n<=b);
 }));
}
