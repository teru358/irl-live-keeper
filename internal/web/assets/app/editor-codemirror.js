import {EditorState} from '@codemirror/state';
import {EditorView, keymap, lineNumbers, drawSelection} from '@codemirror/view';
import {StreamLanguage, syntaxHighlighting, HighlightStyle} from '@codemirror/language';
import {tags} from '@lezer/highlight';
import {defaultKeymap, history, historyKeymap} from '@codemirror/commands';
import {autocompletion, completionKeymap} from '@codemirror/autocomplete';
import {setDiagnostics, lintGutter} from '@codemirror/lint';
import {editorInsertion} from './sources-state.js';
import {bodyAssetInsertion,diagnosticRange} from './source-body.js';
import {templateState, nextTemplateToken, bodyCompletion} from './template-lexer.js';

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
 {tag:[tags.tagName,tags.attributeName,tags.propertyName],color:'var(--act)'},
 {tag:tags.comment,color:'var(--muted)'},
 {tag:tags.keyword,color:'var(--act)'},
 {tag:tags.variableName,color:'var(--source-value)'},
 {tag:tags.string,color:'var(--ok)'},
 {tag:[tags.number,tags.atom],color:'var(--low)'},
 {tag:tags.operator,color:'var(--act)'},
 {tag:tags.invalid,color:'var(--down)'},
]);
export function createEditor(container,{value='',names=[],files=()=>[],onFiles=()=>{},label='',onChange=()=>{},t=key=>key}={}) {
 const phrases=Object.fromEntries([["Completions", "editor_completions"], ["Diagnostics", "editor_diagnostics"], ["No diagnostics", "editor_no_diagnostics"], ["close", "editor_close"], ["Control character", "editor_control_character"]].map(([phrase,key])=>[phrase,t(key)]));
 const completion=context=>{
  return bodyCompletion(context.state.doc.sliceString(0,context.pos),names,files(),context.explicit);
 };
 let cursorPlaced=false;
 const view=new EditorView({parent:container,state:EditorState.create({doc:value,extensions:[
  EditorState.phrases.of(phrases),EditorView.contentAttributes.of({'aria-label':label,'name':'source-editor-template'}),
  EditorView.domEventHandlers({drop(event){if(event.dataTransfer?.files.length){event.preventDefault();event.stopPropagation();onFiles([...event.dataTransfer.files]);return true;}},pointerdown(){cursorPlaced=true;},keydown(){cursorPlaced=true;}}),
  lineNumbers(),EditorView.lineWrapping,drawSelection(),history(),lintGutter(),theme,syntaxHighlighting(colors),
  keymap.of([...completionKeymap,...defaultKeymap,...historyKeymap]),
  template,autocompletion({override:[completion]}),
  EditorView.updateListener.of(update=>{if(update.docChanged)onChange(update.state.doc.toString());}),
 ]})});
 const insert=(text,asset=false)=>{
  const change=(asset?bodyAssetInsertion:editorInsertion)(view.state.doc.toString(),view.state.selection.main,cursorPlaced,text);
  view.dispatch({changes:change,selection:{anchor:change.from+change.insert.length},scrollIntoView:true});
  view.focus();
 };
 return {
  view,
  setValue(text){if(text!==view.state.doc.toString())view.dispatch({changes:{from:0,to:view.state.doc.length,insert:text}});},
  insert,insertAsset:name=>insert(name,true),
  // API positions: 1-based line and Unicode character; CM uses UTF-16.
  setErrors(errors){view.dispatch(setDiagnostics(view.state,errors.map(error=>{
   const line=view.state.doc.line(Math.max(1,Math.min(view.state.doc.lines,error.line)));
   const range=diagnosticRange(line.text,error.column);
   return {from:line.from+range.from,to:line.from+range.to,severity:error.severity||'error',message:error.message};
  })));},
  destroy:()=>view.destroy(),
 };
}
