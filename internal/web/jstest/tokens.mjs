// A helper of the tests (not a test): the whole text through the line lexer of the editor, as {text, style} pieces.
import {templateState,nextTemplateToken} from '../assets/app/template-lexer.js';
export function tokenizeTemplate(text){
 const state=templateState(),tokens=[];
 for(let pos=0;pos<text.length;){const t=nextTemplateToken(text,pos,state);tokens.push({text:text.slice(pos,t.end),style:t.style});pos=t.end;}
 return tokens;
}
