import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const table=JSON.parse(readFileSync(new URL('../../sources/html_cases.json',import.meta.url)));
const rules=JSON.parse(readFileSync(new URL('../../sources/html_rules.json',import.meta.url)));
const module=await import('../assets/app/html.js');
for(const c of table.cases.filter(c=>c.mode==='rendered')) test(c.id,()=>{
 assert.ok(module,'HTML validator module must exist');
 assert.equal(module.validateRendered(c.input,rules)?.code,c.error?.code);
});

const ns='http://www.w3.org/1999/xhtml';
const element=(name,attrs=[],children=[])=>({nodeType:1,localName:name,namespaceURI:ns,attributes:attrs,childNodes:children,textContent:''});
const doc=children=>({head:{childNodes:[]},body:{attributes:[],childNodes:children}});
const attr=(name,value)=>({name,value,namespaceURI:null});
for(const [name,tree,code] of [
 ['namespace',doc([{...element('span'),namespaceURI:'http://www.w3.org/2000/svg'}]),'html_element'],
 ['attribute namespace',doc([element('span',[{...attr('title','x'),namespaceURI:'x'}])]),'html_attribute'],
 ['DOM src',doc([element('img',[attr('src','//evil')])]),'html_url'],
 ['DOM CSS',doc([element('span',[attr('style','background:url(//evil)')])]),'html_url'],
 ['style text',doc([{...element('style'),textContent:'@import "x"'}]),'css_import'],
 ['head',{head:{childNodes:[{}]},body:{attributes:[],childNodes:[]}},'html_nesting'],
 ['body attributes',{head:{childNodes:[]},body:{attributes:[{}],childNodes:[]}},'html_nesting'],
 ['valid',doc([element('span',[attr('title','&literal')])]),null],
]) test('tree '+name,()=>assert.equal(module.validateTree(tree,rules)?.code,code?'template.'+code:undefined));
test('external URL rejected before DOMParser',()=>{
 let calls=0;
 assert.equal(module.parseRendered('<img src="//evil">',rules,{parseFromString(){calls++;}}).error.code,'template.html_url');
 assert.equal(calls,0);
});
test('DOM style NUL',()=>assert.equal(module.validateTree(doc([{...element('style'),textContent:'x\0'}]),rules)?.code,'template.html_nul'));
test('NUL before unclosed literal EOF',()=>{
 for(const input of ['<!--\0','<style>\0']) assert.equal(module.validateRendered(input,rules)?.code,'template.html_nul');
});
