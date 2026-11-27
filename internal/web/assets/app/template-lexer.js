// DOM-free, shared by StreamLanguage and Node tests. Offsets are UTF-16.
export const templateState = () => ({tag:false,format:false});
export function nextTemplateToken(text, pos, state) {
 const rest=text.slice(pos);
 const token=(length,style)=>({end:pos+length,style});
 if(!state.tag){
  if(rest.startsWith('{{')||rest.startsWith('}}'))return token(2,null);
  if(rest[0]==='{'){state.tag=true;return token(1,'bracket');}
  return token(1,rest[0]==='}'?'invalid':null);
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
