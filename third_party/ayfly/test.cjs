// node third_party/ayfly/test.cjs [directory of real test modules]
// Generated fixtures are intentionally music-free (one test tone).
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../..'),wasm=fs.readFileSync(path.join(root,'web/public/audio/ayfly.wasm'));
const formats={pt1:1,pt2:2,pt3:3,stp:4,asc:5,as0:6,psc:7,sqt:8,stc:9,psg:10,ym:11,vtx:12};
const frames=100;
function psg(){return Buffer.from([80,83,71,26,...Array(12).fill(0),0,250,1,0,7,62,8,15,...Array(frames).fill(255),253])}
function ym(){const b=Buffer.alloc(4+14*frames);b.write('YM3!');b.fill(250,4,4+frames);b.fill(62,4+7*frames,4+8*frames);b.fill(15,4+8*frames,4+9*frames);b.fill(255,4+13*frames);return b}
async function decode(data,fmt,{seconds=3,valid=true,full=false}={}){
 const {instance}=await WebAssembly.instantiate(fmt===3?fs.readFileSync(path.join(root,'web/public/audio/pt3.wasm')):wasm);
 const w=instance.exports;new Uint8Array(w.memory.buffer,w.rr_input(),data.length).set(data);
 if(!valid){try{assert.notEqual(w.rr_init(data.length,44100,fmt),1)}catch(e){if(!(e instanceof WebAssembly.RuntimeError))throw e}return}
 assert.equal(w.rr_init(data.length,44100,fmt),1);let samples=0,energy=0;
 const limit=44100*(full?1801:seconds);
 while(samples<limit&&!w.rr_ended()){
  const n=w.rr_render(16384);assert(n>=0&&n<=16384);assert(n||w.rr_ended());const pcm=new Float32Array(w.memory.buffer,w.rr_output(),n*2);
  for(const v of pcm){assert(Number.isFinite(v));energy+=v*v}samples+=n;
 }
 assert(samples>44100,'too short');assert(energy/samples>1e-6,'silent');
 if(full)assert.equal(w.rr_ended(),1,'did not finish');
 return {seconds:(samples/44100).toFixed(2),rms:Math.sqrt(energy/(samples*2)).toFixed(4),ended:!!w.rr_ended()};
}
(async()=>{
 for(const [name,b,fmt] of [['PSG tone',psg(),10],['YM3 tone',ym(),11]]){const r=await decode(b,fmt,{full:true});assert.equal(Number(r.seconds),2);console.log('PASS',name,r)}
 for(const fmt of Object.values(formats).filter(n=>n!==3))await decode(Buffer.alloc(16),fmt,{valid:false});
 const args=process.argv.slice(2);if(args.length){for(const name of fs.readdirSync(args[0])){const fmt=formats[path.extname(name).slice(1).toLowerCase()];if(!fmt)continue;console.log('PASS',name,await decode(fs.readFileSync(path.join(args[0],name)),fmt,{seconds:10,full:args.includes('--full')}))}}
 console.log('AY decoder tests passed');
})().catch(e=>{console.error(e);process.exitCode=1});
