import {EditorState} from '@codemirror/state';
import {EditorView, keymap, lineNumbers, drawSelection} from '@codemirror/view';
import {StreamLanguage, syntaxHighlighting, HighlightStyle} from '@codemirror/language';
import {tags} from '@lezer/highlight';
import {defaultKeymap, history, historyKeymap} from '@codemirror/commands';
import {autocompletion, completionKeymap} from '@codemirror/autocomplete';
import {setDiagnostics, lintGutter} from '@codemirror/lint';
import {css} from '@codemirror/lang-css';
import {templateState, nextTemplateToken, tokenizeTemplate} from './template-lexer.js';

const template=StreamLanguage.define({
 startState:templateState,
 token(stream,state){const t=nextTemplateToken(stream.string,stream.pos,state);stream.pos=t.end;return t.style;},
});
const theme=EditorView.theme({
 '&':{color:'var(--ink)',backgroundColor:'var(--surface)',border:'1px solid var(--line)'},
 '.cm-content':{caretColor:'var(--ink)'},
 '.cm-cursor':{borderLeftColor:'var(--ink)'},
 '.cm-gutters':{color:'var(--muted)',backgroundColor:'var(--bg)',borderColor:'var(--line)'},
 '.cm-selectionBackground, &.cm-focused .cm-selectionBackground':{backgroundColor:'var(--track)'},
 '.cm-tooltip':{color:'var(--ink)',backgroundColor:'var(--surface)',borderColor:'var(--line)'},
 '.cm-tooltip-autocomplete ul li[aria-selected]':{backgroundColor:'var(--act)',color:'var(--act-ink)'},
 '.cm-diagnostic':{borderColor:'var(--down)'},
});
const colors=HighlightStyle.define([
 {tag:[tags.bracket,tags.punctuation],color:'var(--muted)'},
 {tag:tags.keyword,color:'var(--act)'},
 {tag:tags.variableName,color:'var(--ink)'},
 {tag:tags.string,color:'var(--ok)'},
 {tag:[tags.number,tags.atom],color:'var(--low)'},
 {tag:tags.operator,color:'var(--act)'},
 {tag:tags.invalid,color:'var(--down)'},
]);
export function createEditor(container,{kind='template',value='',names=[],label='',onChange=()=>{},t=key=>key}={}) {
 const phrases=Object.fromEntries([["Completions", "editor_completions"], ["Diagnostics", "editor_diagnostics"], ["No diagnostics", "editor_no_diagnostics"], ["close", "editor_close"], ["Control character", "editor_control_character"]].map(([phrase,key])=>[phrase,t(key)]));
 const completion=context=>{
  const prefix=context.state.doc.sliceString(0,context.pos);
  const tokens=tokenizeTemplate(prefix);
  const state=templateState();
  for(let p=0;p<prefix.length;){p=nextTemplateToken(prefix,p,state).end;}
  const word=context.matchBefore(/[A-Za-z_][A-Za-z0-9_.]*$/);
  const last=tokens.at(-1);
  // 打ちかけの名前（`link.`）の最後の `.` は字句では誤りになるが、補完は出す
  if(!state.tag||state.format||last?.style==='string')return null;
  if(last?.style==='invalid'&&!(word&&word.text.endsWith('.')))return null;
  if(!word&&!context.explicit)return null;
  return {from:word?word.from:context.pos,options:names.map(name=>({label:name,type:'variable'})),validFor:/^[A-Za-z_][A-Za-z0-9_.]*$/};
 };
 const view=new EditorView({parent:container,state:EditorState.create({doc:value,extensions:[
  EditorState.phrases.of(phrases),EditorView.contentAttributes.of({'aria-label':label}),
  lineNumbers(),drawSelection(),history(),lintGutter(),theme,syntaxHighlighting(colors),
  keymap.of([...completionKeymap,...defaultKeymap,...historyKeymap]),
  kind==='css'?css():[template,autocompletion({override:[completion]})],
  EditorView.updateListener.of(update=>{if(update.docChanged)onChange(update.state.doc.toString());}),
 ]})});
 const insert=text=>{view.dispatch(view.state.replaceSelection(text),{scrollIntoView:true});view.focus();};
 return {
  view,getValue:()=>view.state.doc.toString(),
  setValue(text){if(text!==view.state.doc.toString())view.dispatch({changes:{from:0,to:view.state.doc.length,insert:text}});},
  insert,insertValue:name=>insert('{'+name+'}'),
  insertCondition:(condition='',body='')=>insert('{if '+condition+'}'+body+'{end}'),
  // API positions: 1-based line and Unicode character; CM uses UTF-16.
  setErrors(errors){view.dispatch(setDiagnostics(view.state,errors.map(error=>{
   const line=view.state.doc.line(Math.max(1,Math.min(view.state.doc.lines,error.line)));
   const offset=Array.from(line.text).slice(0,Math.max(0,error.column-1)).join('').length;
   const from=line.from+offset;
   return {from,to:Math.min(line.to,from+(line.text.codePointAt(offset)>0xffff?2:1)),severity:'error',message:error.message};
  })));},
  destroy:()=>view.destroy(),
 };
}
