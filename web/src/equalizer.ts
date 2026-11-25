// Shared DSP for browser audio and the AY decoder, with true channel balance.
export const frequencies=[60,170,310,600,1000,3000,6000,12000,14000,16000];
export type EQSettings={enabled:boolean,preamp:number,bands:number[],balance:number};
export const flat=():EQSettings=>({enabled:false,preamp:0,bands:Array(10).fill(0),balance:0});
const db=(v:number)=>Math.pow(10,v/20);
const clamp=(v:number,min:number,max:number)=>Number.isFinite(v)?Math.max(min,Math.min(max,v)):0;
export function sanitizeEQ(v:Partial<EQSettings>|undefined):EQSettings{return {enabled:v?.enabled===true,preamp:clamp(Number(v?.preamp??0),-12,12),bands:frequencies.map((_,i)=>clamp(Number(v?.bands?.[i]??0),-12,12)),balance:clamp(Number(v?.balance??0),-1,1)}}
export class AudioEffects{
 readonly input:GainNode;readonly analyser:AnalyserNode;
 private pre:GainNode;private filters:BiquadFilterNode[];private split:ChannelSplitterNode;private left:GainNode;private right:GainNode;private merge:ChannelMergerNode;private master:GainNode;
 constructor(private ctx:BaseAudioContext,destination:AudioNode=ctx.destination){
  this.input=ctx.createGain();this.input.channelCount=2;this.input.channelCountMode='explicit';this.input.channelInterpretation='speakers';this.pre=ctx.createGain();this.input.connect(this.pre);
  this.filters=frequencies.map(f=>{const n=ctx.createBiquadFilter();n.type='peaking';n.frequency.value=Math.min(f,ctx.sampleRate*.475);n.Q.value=1.4;return n});let previous:AudioNode=this.pre;for(const n of this.filters){previous.connect(n);previous=n}
  this.split=ctx.createChannelSplitter(2);this.left=ctx.createGain();this.right=ctx.createGain();this.merge=ctx.createChannelMerger(2);this.master=ctx.createGain();this.analyser=ctx.createAnalyser();this.analyser.fftSize=2048;this.analyser.smoothingTimeConstant=.65;
  previous.connect(this.split);this.split.connect(this.left,0);this.split.connect(this.right,1);this.left.connect(this.merge,0,0);this.right.connect(this.merge,0,1);this.merge.connect(this.master);this.master.connect(this.analyser);this.analyser.connect(destination);
 }
 update(s:EQSettings,volume:number,immediate=false){const set=(p:AudioParam,v:number)=>{if(immediate)p.value=v;else p.setTargetAtTime(v,this.ctx.currentTime,.015)};set(this.pre.gain,s.enabled?db(s.preamp):1);this.filters.forEach((n,i)=>set(n.gain,s.enabled?s.bands[i]:0));set(this.left.gain,s.balance>0?1-s.balance:1);set(this.right.gain,s.balance<0?1+s.balance:1);set(this.master.gain,clamp(volume,0,1))}
 dispose(){for(const n of [this.input,this.pre,...this.filters,this.split,this.left,this.right,this.merge,this.master,this.analyser])n.disconnect()}
}
