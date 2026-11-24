import butterchurn from 'butterchurn';
import pack from 'butterchurn-presets';
import {convertPreset} from 'milkdrop-preset-converter';
const canvas=document.getElementById('milk') as HTMLCanvasElement;
const presets=pack.getPresets(),custom=new Map<string,unknown>();let visual:ReturnType<typeof butterchurn.createVisualizer>|undefined;
let selected='',busy=false,disabled=false;
const send=(type:string,extra:Record<string,unknown>={})=>parent.postMessage({type,...extra},'*');
function size(){return {w:Math.max(32,Math.min(1280,Math.round(innerWidth))),h:Math.max(32,Math.min(720,Math.round(innerHeight)))}}
function init(){const {w,h}=size();visual=butterchurn.createVisualizer(null,canvas,{width:w,height:h,pixelRatio:1,textureRatio:1,meshWidth:32,meshHeight:24});}
function load(name:string){if(!visual)return;const preset=custom.get(name)??presets[name];if(!preset)throw Error('Preset not found');visual.loadPreset(preset,selected?1.2:0);selected=name;send('selected',{name})}
window.addEventListener('resize',()=>{if(visual){const {w,h}=size();visual.setRendererSize(w,h)}});
window.addEventListener('message',async e=>{if(e.source!==parent||!e.data||typeof e.data.type!=='string'||disabled)return;const d=e.data;try{
 if(d.type==='preset'&&typeof d.name==='string')load(d.name);
 if(d.type==='import'&&!busy&&typeof d.name==='string'&&typeof d.text==='string'&&d.text.length<=262144&&custom.size<100){busy=true;try{const p=await convertPreset(d.text);custom.set(d.name,p);load(d.name);send('imported',{name:d.name})}finally{busy=false}}
 if(d.type==='audio'&&visual&&d.wave instanceof Uint8Array&&d.wave.length===1024){visual.render({audioLevels:{timeByteArray:d.wave,timeByteArrayL:d.wave,timeByteArrayR:d.wave}});send('frame')}
 }catch(err){if(d.type==='audio')disabled=true;send('error',{message:String(err).slice(0,250)})}});
try{init();send('ready',{names:Object.keys(presets).sort()})}catch(e){send('error',{message:'WebGL 2: '+String(e)})}
