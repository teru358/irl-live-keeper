import {test} from 'node:test';
import assert from 'node:assert/strict';
import {reconcile,updateRendered} from '../assets/app/reconcile.js';
class Node {
 constructor(name,text='',attrs={}){this.nodeType=name?'1':3;this.nodeType=Number(this.nodeType);this.localName=name;this.textContent=text;this.attrs={...attrs};this.childNodes=[];this.ownerDocument={importNode:n=>n.cloneNode(true)};this.writes=[];}
 get attributes(){return Object.entries(this.attrs).map(([name,value])=>({name,value}));}
 getAttribute(k){return this.attrs[k]??null;}
 setAttribute(k,v){this.writes.push(k);this.attrs[k]=v;}
 removeAttribute(k){delete this.attrs[k];}
 append(...nodes){this.childNodes.push(...nodes);}
 insertBefore(n,ref){this.childNodes=this.childNodes.filter(x=>x!==n);const i=this.childNodes.indexOf(ref);this.childNodes.splice(i<0?this.childNodes.length:i,0,n);}
 removeChild(n){this.childNodes.splice(this.childNodes.indexOf(n),1);}
 cloneNode(){const n=new Node(this.localName,this.textContent,this.attrs);n.append(...this.childNodes.map(c=>c.cloneNode()));return n;}
}
const el=(name,attrs={},text='')=>new Node(name,text,attrs);
test('identity uses id, otherwise position and tag; updates attributes and text',()=>{
 const root=el('div'), a=el('span',{id:'a',title:'old'}),b=el('b');a.append(new Node(null,'old'));root.append(a,b);
 const next=el('body'),n=el('span',{id:'a',class:'new'});n.append(new Node(null,'new'));next.append(el('i'),n);
 reconcile(root,next);assert.equal(root.childNodes[1],a);assert.equal(a.getAttribute('title'),null);assert.equal(a.childNodes[0].textContent,'new');assert.equal(a.getAttribute('class'),'new');
 const same=el('body');same.append(el('i'),n);const first=root.childNodes[0];reconcile(root,same);assert.equal(root.childNodes[0],first);
});
test('style and image survive unchanged; image src updates without replacement',()=>{
 const root=el('div'),style=el('style',{},'x{}'),img=el('img',{src:'/source/files/a.gif'});root.append(style,img);
 const next=el('body');next.append(style.cloneNode(),img.cloneNode());reconcile(root,next);assert.equal(root.childNodes[0],style);assert.equal(root.childNodes[1],img);assert.deepEqual(img.writes,[]);
 next.childNodes[1].attrs.src='/source/files/a.gif?v=123456789abc';reconcile(root,next);assert.equal(root.childNodes[1],img);assert.deepEqual(img.writes,['src']);
});
test('invalid rendered string preserves previous display before parsing',()=>{
 const root=el('div');root.append(el('b'));let called=false;
 const rules={elements:[]};assert.equal(updateRendered(root,'<script></script>',rules,{parseFromString(){called=true;}}),false);assert.equal(called,false);assert.equal(root.childNodes[0].localName,'b');
});
test('duplicate ids consume separate siblings and comments update',()=>{
 const root=el('div'),a=el('b',{id:'same'}),b=el('b',{id:'same'}),comment=new Node(null,'old');comment.nodeType=8;root.append(a,b,comment);
 const next=el('body'),c=new Node(null,'new');c.nodeType=8;next.append(el('b',{id:'same',title:'one'}),el('b',{id:'same',title:'two'}),c);
 reconcile(root,next);assert.equal(root.childNodes[0],a);assert.equal(root.childNodes[1],b);assert.equal(a.getAttribute('title'),'one');assert.equal(b.getAttribute('title'),'two');assert.equal(root.childNodes[2],comment);assert.equal(comment.textContent,'new');
});

for (const scenario of ['tag change','removal','insertion']) test('retained sibling is never moved after preceding '+scenario,()=>{
 const root=el('div'),keep=el('span',{id:'keep'}),next=el('body');
 root.append(el('div'),keep);
 if(scenario==='tag change')next.append(el('p'));
 if(scenario==='insertion')next.append(el('div'),el('b'));
 next.append(keep.cloneNode());
 const calls=[];
 for(const method of ['insertBefore','appendChild','removeChild']){
  const original=root[method];
  root[method]=function(node,...args){if(node===keep)calls.push(method);return original?.call(this,node,...args);};
 }
 reconcile(root,next);
 assert.equal(root.childNodes.at(-1),keep);assert.deepEqual(calls,[]);
});
