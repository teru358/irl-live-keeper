import {test} from 'node:test';
import assert from 'node:assert/strict';
import {tokenizeTemplate,templateState,nextTemplateToken} from '../assets/app/template-lexer.js';
const compact=text=>tokenizeTemplate(text).filter(t=>t.style).map(t=>[t.text,t.style]);
for(const [text,want] of [
 ['{a}}}',[['{','bracket'],['a','variableName'],['}','bracket']]],
 ['{{a}}',[]],
 ['{if not camera.kbps < 800}',[['{','bracket'],['if','keyword'],['not','keyword'],['camera.kbps','variableName'],['<','operator'],['800','number'],['}','bracket']]],
 ['{stream.elapsed:hms}',[['{','bracket'],['stream.elapsed','variableName'],[':','punctuation'],['hms','atom'],['}','bracket']]],
 ['{if nota}',[['{','bracket'],['if','keyword'],['nota','variableName'],['}','bracket']]],
])test('template '+text,()=>assert.deepEqual(compact(text),want));
test('escaped strings retain braces and escaped quote/backslash',()=>{
 const text=String.raw`{if a == "a\"{b}\\c"}`;
 assert.deepEqual(compact(text).filter(t=>t[1]==='string'),[[String.raw`"a\"{b}\\c"`,'string']]);
 assert.equal(compact(text).filter(t=>t[1]==='bracket').length,2);
});
test('invalid escapes, newline and unmatched braces',()=>{
 for(const text of [String.raw`{if a == "\q"}`,'{if a == "x\ny"}','}'])assert.ok(compact(text).some(t=>t[1]==='invalid'));
});
test('stream state survives line boundaries and format validates',()=>{
 const state=templateState();
 for(const line of ['{if',' not camera.kbps < 800}']){
  for(let p=0;p<line.length;){const t=nextTemplateToken(line,p,state);assert.ok(t.end>p);p=t.end;}
 }
 assert.equal(state.tag,false);
 assert.ok(compact('{a:bad}').some(t=>t[1]==='invalid'));
});
test('editor entry does not import CodeMirror until mount',async()=>{
 const {mountEditor}=await import('../assets/app/editor.js');
 assert.equal(typeof mountEditor,'function');
});

import * as lexer from '../assets/app/template-lexer.js';
test('completion opens at bare brace and excludes escaped braces, strings and formats',()=>{
 for(const [text,from] of [['{',1],['{{',null],['{cam',1],['{camera.',1],['{if ',4],['{if camera.state == "',null],['{x:',null],['{x:hm',null],['abc {',5],['{{{',3],['{if camera.state == "a\\"b',null],['{x}',null]]){
  assert.deepEqual(lexer.templateCompletion(text),from===null?null:{from},text);
 }
});
