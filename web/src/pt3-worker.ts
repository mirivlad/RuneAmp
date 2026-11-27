import {splitTurboSound,validSinglePT3} from './turbosound.ts';
type WasmAY={memory:WebAssembly.Memory;rr_input:()=>number;rr_output:()=>number;rr_init:(length:number,rate:number,format:number)=>number;rr_render:(n:number)=>number;rr_ended:()=>number};
const worker=self as unknown as {onmessage:(e:MessageEvent)=>void;postMessage:(data:unknown,transfer?:Transferable[])=>void};
let chips:WasmAY[]=[];
worker.onmessage=async({data})=>{try{
 if(data.type==='init'){
  const file=new Uint8Array(data.file),format=data.format;
  if(!Number.isInteger(format)||format<1||format>13||file.length<16||file.length>(format===3||format===13?131088:format>=10?2*1024*1024:65536))throw Error('Неподдерживаемый размер или формат модуля.');
  const pt3=format===3||format===13,parts=pt3?splitTurboSound(file):undefined;
  if(format===13&&!parts)throw Error('Не найден контейнер 02TS.');
  const modules=parts??[file];
  if(pt3&&modules.some(b=>!validSinglePT3(b)))throw Error('Неподдерживаемый или повреждённый PT3.');
  const response=await fetch(data.wasmUrl);if(!response.ok)throw Error('Модуль проигрывателя не найден.');
  const compiled=await WebAssembly.compile(await response.arrayBuffer());
  chips=[];
  for(const b of modules){
   const instance=await WebAssembly.instantiate(compiled),w=instance.exports as unknown as WasmAY;
   new Uint8Array(w.memory.buffer,w.rr_input(),b.length).set(b);
   if(w.rr_init(b.length,data.rate,pt3?3:format)!==1)throw Error('Не удалось распознать музыкальный модуль.');
   chips.push(w);
  }
  worker.postMessage({type:'ready',chips:chips.length});return;
 }
 if(data.type==='chunk'&&chips.length){
  const mixed=new Float32Array(16384*2);let count=0;
  for(const w of chips){
   if(w.rr_ended())continue;
   const n=w.rr_render(16384);if(n<0||n>16384)throw Error('Ошибка декодера.');
   const pcm=new Float32Array(w.memory.buffer,w.rr_output(),n*2);
   for(let i=0;i<pcm.length;i++){if(!Number.isFinite(pcm[i]))throw Error('Ошибка декодирования.');mixed[i]+=pcm[i]/chips.length}
   count=Math.max(count,n);
  }
  const samples=mixed.slice(0,count*2);
  worker.postMessage({type:'chunk',samples,ended:chips.every(w=>!!w.rr_ended())},[samples.buffer])
 }
 }catch(e){chips=[];worker.postMessage({type:'error',message:e instanceof Error?e.message:'Ошибка AY.'})}};
