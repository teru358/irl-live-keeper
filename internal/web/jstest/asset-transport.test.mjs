import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';
const source=readFileSync(new URL('../assets/app/sources.js',import.meta.url),'utf8');
// Execute the actual screen handlers with their surrounding UI dependencies supplied.
function handler(name,next,context){
 const start=source.indexOf(' async function '+name+'('),end=source.indexOf(next,start);
 assert.ok(start>=0&&end>start);
 return runInNewContext('('+source.slice(start,end).trim()+')',context);
}
test('upload handler sends NFD filename as NFC in FormData, through request',async()=>{
 let sent;
 const upload=handler('uploadFiles',' async function upload(',{
 generation:1,FormData,request:async(path,body)=>{sent={path,body};return {name:'が.svg'};},
 load:async()=>{},editor:null,fail:e=>{throw e;},t:x=>x,
 });
 const file=new Blob(['<svg/>'],{type:'image/svg+xml'});Object.defineProperty(file,'name',{value:'か\u3099.svg'});
 await upload([file]);assert.equal(sent.path,'source-files');assert.equal(sent.body.get('file').name,'が.svg');assert.equal(await sent.body.get('file').text(),'<svg/>');
});
test('delete handler encodes Japanese paths by segment on the wire',async()=>{
 let sent;
 const remove=handler('removeFile',' function insertBody(',{
 askDelete:async()=>true,store:{get:()=>({list:[]})},t:x=>x,update:()=>{},load:async()=>{},fail:e=>{throw e;},
 request:async(...args)=>{sent=args;},
 });
 await remove({name:'親/夕焼け.svg',used_by:[]});
 assert.equal(sent[0],'source-files/%E8%A6%AA/%E5%A4%95%E7%84%BC%E3%81%91.svg');assert.equal(sent[2],'DELETE');
});
test('unusable rows offer deletion only for name collisions',()=>{
 const start=source.indexOf('<div class="source-file unusable">');
 const template=source.slice(start,source.indexOf("</div>",start)+6);
 assert.ok(template);
 const render=runInNewContext('(f=>html`'+template+'`)',{
 html:(parts,...values)=>parts.reduce((out,part,i)=>out+part+(values[i]??''),''),
 t:x=>x,removeFile:()=>{},
 });
 assert.match(render({name:'A.svg',reason:'name_collision',reason_text:'collision'}),/<button/);
 for(const reason of ['bad_name','bad_extension','unreadable','over_limit']){
 assert.doesNotMatch(render({name:'x.svg',reason,reason_text:reason}),/<button/);
 }
});
