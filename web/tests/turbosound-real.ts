// Optional integration test, using the operator's own collection. No music is bundled.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {splitTurboSound} from '../src/turbosound.ts';
const dir=process.argv[2];
if(!dir)throw Error('Pass the path to a TurboSound collection');
const wasm=fs.readFileSync(new URL('../public/audio/pt3.wasm',import.meta.url));
let reply:any;
(globalThis as any).self={postMessage:(data:unknown)=>{reply=data}};
globalThis.fetch=async()=>new Response(wasm);
await import('../src/pt3-worker.ts');
const dispatch=async(data:unknown)=>{reply=undefined;await (globalThis as any).self.onmessage({data});assert(reply);if(reply.type==='error')throw Error(reply.message);return reply};
let passed=0,failed=0,legacy=0,container=0;
for(const name of fs.readdirSync(dir,{recursive:true,encoding:'utf8'})){
 const file=path.join(dir,name);
 if(!/\.(ts|pt3)$/i.test(file)||!fs.statSync(file).isFile())continue;
 try{
  const bytes=fs.readFileSync(file);
  const parts=splitTurboSound(bytes);
  if(!parts)continue;
  const ready=await dispatch({type:'init',file:Uint8Array.from(bytes),format:3,rate:44100,wasmUrl:'test'});
  assert.equal(ready.chips,2);
  let energy=0,frames=0,peak=0;
  for(let i=0;i<8;i++){
   const r=await dispatch({type:'chunk'});frames+=r.samples.length/2;
   for(const v of r.samples){assert(Number.isFinite(v));energy+=v*v;peak=Math.max(peak,Math.abs(v))}
   if(r.ended)break;
  }
  assert(frames>0&&energy>0,'silent or empty');
  assert(peak<=1,'clipping');
  passed++;if(bytes.subarray(-4).toString()==='02TS')container++;else legacy++;
 }catch(e){failed++;console.error('FAIL',name,String(e))}
}
console.log({passed,container,legacy,failed});
assert(passed>0&&failed===0);
