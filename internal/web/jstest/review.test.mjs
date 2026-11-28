import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
test('preview skips identical HTML and name, still resizes and redraws changed content',async()=>{
 const source=readFileSync(new URL('../assets/app/sources.js',import.meta.url),'utf8');
 const fn=source.slice(source.indexOf(' function previewFrame('),source.indexOf(' async function load('));
 let updates=0,load;
 const box={};
 const frame={style:{},setAttribute(){},addEventListener(_,f){load=f;},contentDocument:{querySelector(){return box;}}};
 const host={isConnected:true,clientWidth:100,clientHeight:100,append(){}};
 const preview=new Function('frames','activeFrames','document','ResizeObserver','rulesPromise','updateRendered','DOMParser','t',fn+';return previewFrame;')(
 new WeakMap(),new Set(),{createElement(){return frame;}},class{observe(){} disconnect(){}},Promise.resolve({}),()=>{updates++;return true;},class{},x=>x);
 preview(host,'hello','a');load();await new Promise(r=>setImmediate(r));
 assert.equal(updates,1);
 preview(host,'hello','a',200,200);await new Promise(r=>setImmediate(r));
 assert.equal(updates,1);assert.equal(frame.style.width,'200px');
 preview(host,'hello','b');await new Promise(r=>setImmediate(r));assert.equal(updates,2);
 preview(host,'changed','b');await new Promise(r=>setImmediate(r));assert.equal(updates,3);
});
test('source retries rules after network and HTTP errors then displays events',async()=>{
 const source=readFileSync(new URL('../assets/source.js',import.meta.url),'utf8').replace(/^import .*;\n/,'');
 let attempts=0,waits=0,updates=0,events;
 const run=new (Object.getPrototypeOf(async function(){}).constructor)('document','fetch','DOMParser','EventSource','window','setTimeout','updateRendered',source);
 await run({querySelector(){return {};}},async()=>{
  attempts++;if(attempts===1)throw Error('offline');
  return {ok:attempts>2,json:async()=>({})};
 },class{},class{constructor(){events=this;}},{location:{pathname:'/source/demo'}},f=>{waits++;f();},()=>updates++);
 events.onmessage({data:'"hello"'});assert.equal(attempts,3);assert.equal(waits,2);assert.equal(updates,1);
});

test('detached preview observers disconnect and leave the active set',()=>{
 const source=readFileSync(new URL('../assets/app/sources.js',import.meta.url),'utf8');
 const start=source.indexOf(' function cleanupFrames()');
 const fn=source.slice(start,source.indexOf(' const rulesPromise',start));
 let disconnected=0;
 const detached={host:{isConnected:false},observer:{disconnect(){disconnected++;}}};
 const connected={host:{isConnected:true},observer:{disconnect(){throw Error('connected');}}};
 const active=new Set([detached,connected]);
 new Function('activeFrames',fn+';cleanupFrames();')(active);
 assert.equal(disconnected,1);assert.deepEqual([...active],[connected]);
});
