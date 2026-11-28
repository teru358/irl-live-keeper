import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {tokenizeTemplate,templateCompletion,bodyCompletion,insideStyle} from '../assets/app/template-lexer.js';
import {sourceTemplates,templateBody,bodyAssetInsertion,fileAccept} from '../assets/app/source-body.js';
test('HTML attributes and template precedence',()=>{
 const ts=tokenizeTemplate('<span class="{if cam1.kbps < 8}low{end}">文字</span>');
 assert.ok(ts.some(t=>t.text==='span'&&t.style==='tagName'));
 assert.ok(ts.some(t=>t.text==='class'&&t.style==='attributeName'));
 assert.ok(ts.some(t=>t.text==='cam1.kbps'&&t.style==='variableName'));
});
test('raw style and comments suppress substitutions and completion',()=>{
 for(const text of ['<!-- {cam1.kbps} -->','<STYLE>.x { color:red; content:"{cam1.name}"; }</STYLE >']){
  assert.ok(!tokenizeTemplate(text).some(t=>t.style==='variableName'));
 }
 for(const prefix of ['<!-- {cam','<style>{cam'])assert.equal(templateCompletion(prefix),null);
 assert.equal(insideStyle('<style>/* </stylesheet> */'),true);
 assert.equal(insideStyle('<style>"</STYLE >"'),false);
 assert.equal(insideStyle('<!-- <style> -->'),false);
});
test('completion values and usable nested paths',()=>{
 const files=[{name:'a/b.png',usable:true},{name:'bad.png',usable:false}];
 assert.deepEqual(bodyCompletion('{cam',['cam1.kbps'],files).options.map(x=>x.label),['cam1.kbps']);
 assert.deepEqual(bodyCompletion('<img src="/source/files/a/',[],files).options.map(x=>x.label),['a/b.png']);
});
test('four templates use lowest numeric camera id or retain cam1',()=>{
 assert.equal(sourceTemplates.length,4);
 for(let i=0;i<4;i++){
  assert.ok(templateBody(i,[{id:'cam8'},{id:'cam2'}]).includes('cam2.'));
  assert.ok(templateBody(i,[]).includes('cam1.'));
 }
});
test('asset insertion uses cursor context and preserves nested paths',()=>{
 assert.equal(bodyAssetInsertion('<style>',{from:7,to:7},true,'a/b.png').insert,'url("/source/files/a/b.png")');
 assert.equal(bodyAssetInsertion('abc',{from:0,to:0},false,'a/b.png').from,3);
 assert.equal(bodyAssetInsertion('abc',{from:0,to:1},true,'x.svg').insert,'<img src="/source/files/x.svg">');
});
test('accept matches Go extension allowlist',()=>{
 const go=readFileSync(new URL('../../sources/files.go',import.meta.url),'utf8');
 const check=go.slice(go.indexOf('if ext !='),go.indexOf('stem :='));
 assert.deepEqual(fileAccept.split(',').sort(),[...check.matchAll(/ext != "(\.[a-z]+)"/g)].map(m=>m[1]).sort());
});
import {diagnosticRange} from '../assets/app/source-body.js';
import {applyPreview} from '../assets/app/sources-state.js';
test('diagnostics use Unicode columns and style warnings follow preview',()=>{
 assert.deepEqual(diagnosticRange('😀あx',2),{from:2,to:3});
 const state={draft:{body:''}};
 applyPreview(state,{text:'x',warnings:[{code:'template.style_literal',message:'literal'}]});
 assert.equal(state.warnings[0].code,'template.style_literal');
 applyPreview(state,{template_error:{line:2,column:3,error:'bad'}});
 assert.deepEqual(state.diagnostics,[{line:2,column:3,message:'bad'}]);
 assert.deepEqual(state.warnings,[]);
});
