export const modes=['spectrum','scope','radial','waterfall','plasma','stars'] as const;
type Mode=typeof modes[number];
type Palette={bg:string,low:string,mid:string,high:string};
const palettes:Palette[]=[
 {bg:'#080b12',low:'#3657ad',mid:'#70b5ff',high:'#e2f4ff'},
 {bg:'#130e08',low:'#a46a27',mid:'#efb849',high:'#fff0b4'},
 {bg:'#031008',low:'#217a42',mid:'#72eb8d',high:'#e3ffbb'},
 {bg:'#100719',low:'#7442aa',mid:'#dd79ed',high:'#ffdcfc'},
 {bg:'#121212',low:'#666',mid:'#bbb',high:'#fff'}
];
export class Visualizer {
 private freq=new Uint8Array(1024);private wave=new Uint8Array(2048);private peak=new Float32Array(64);private bars=new Float32Array(64);
 private mini:CanvasRenderingContext2D;private large:CanvasRenderingContext2D;private frame=0;private last=0;private phase=0;private energy=0;
 private waterfall=document.createElement('canvas');private waterCtx:CanvasRenderingContext2D;
 private trail=document.createElement('canvas');private trailCtx:CanvasRenderingContext2D;
 mode:Mode='spectrum';palette=0;miniPalette=1;
 constructor(private small:HTMLCanvasElement,private big:HTMLCanvasElement,private source:()=>AnalyserNode|undefined){
  this.mini=small.getContext('2d')!;this.large=big.getContext('2d')!;this.waterfall.width=320;this.waterfall.height=180;this.waterCtx=this.waterfall.getContext('2d')!;this.trail.width=640;this.trail.height=360;this.trailCtx=this.trail.getContext('2d')!;
 }
 setMode(mode:string){if(modes.includes(mode as Mode)){this.mode=mode as Mode;this.clear()}}
 setPalette(n:number){if(Number.isInteger(n)&&palettes[n]){this.palette=n;this.clear()}}
 private clear(){this.waterCtx.clearRect(0,0,320,180);this.trailCtx.clearRect(0,0,640,360)}
 start(){if(!this.frame)this.frame=requestAnimationFrame(this.draw)}
 private draw=(time:number)=>{this.frame=requestAnimationFrame(this.draw);if(document.hidden||(!this.small.getClientRects().length&&!this.big.getClientRects().length))return;if(time-this.last<30)return;const dt=Math.min(.1,(time-this.last)/1000);this.last=time;
 const a=this.source();this.freq.fill(0);this.wave.fill(128);if(a){a.getByteFrequencyData(this.freq);a.getByteTimeDomainData(this.wave)}const bins=a?.frequencyBinCount??1024;
 for(let i=0;i<64;i++){const lo=Math.max(1,Math.floor(Math.pow(bins*.8,i/64))),hi=Math.max(lo+1,Math.floor(Math.pow(bins*.8,(i+1)/64)));let v=0;for(let j=lo;j<Math.min(hi,bins);j++)v=Math.max(v,this.freq[j]/255);this.bars[i]=Math.max(v,this.bars[i]-dt*1.8);this.peak[i]=Math.max(this.bars[i],this.peak[i]-dt*.45)}
 this.energy=this.energy*.8+(this.bars.slice(0,18).reduce((x,y)=>x+y,0)/18)*.2;this.phase+=dt*(.4+this.energy*2);
 if(this.small.getClientRects().length){const selected=this.palette;this.palette=this.miniPalette;this.spectrum(this.mini,this.small.width,this.small.height,true);this.palette=selected;}
 if(this.big.getClientRects().length){const rect=this.big.getBoundingClientRect(),ratio=Math.min(2,devicePixelRatio||1),w=Math.max(1,Math.min(1920,Math.round(rect.width*ratio))),h=Math.max(1,Math.min(1080,Math.round(rect.height*ratio)));if(this.big.width!==w||this.big.height!==h){this.big.width=w;this.big.height=h}this.render(this.large,w,h,dt)}
 }
 private get p(){return palettes[this.palette]}
 private background(c:CanvasRenderingContext2D,w:number,h:number){c.fillStyle=this.p.bg;c.fillRect(0,0,w,h)}
 private spectrum(c:CanvasRenderingContext2D,w:number,h:number,mini=false){this.background(c,w,h);const count=mini?32:64,step=w/count,segment=mini?4:Math.max(4,h/38);for(let i=0;i<count;i++){const index=Math.floor(i*64/count),v=this.bars[index],height=v*(h-8);for(let y=0;y<height;y+=segment){c.fillStyle=y/h>.76?this.p.high:y/h>.38?this.p.mid:this.p.low;c.fillRect(i*step+1,h-y-segment,Math.max(1,step-2),segment-1)}c.fillStyle=this.p.high;c.fillRect(i*step+1,h-2-this.peak[index]*(h-8),Math.max(1,step-2),mini?1:2)}}
 private render(c:CanvasRenderingContext2D,w:number,h:number,dt:number){if(this.mode==='spectrum'){this.spectrum(c,w,h);return}this.background(c,w,h);const cx=w/2,cy=h/2,r=Math.min(w,h)*.22,p=this.p;
 if(this.mode==='scope'){c.strokeStyle=p.low;c.lineWidth=1;for(let i=1;i<8;i++){c.beginPath();c.moveTo(w*i/8,0);c.lineTo(w*i/8,h);c.stroke()}for(let i=1;i<6;i++){c.beginPath();c.moveTo(0,h*i/6);c.lineTo(w,h*i/6);c.stroke()}c.strokeStyle=p.high;c.lineWidth=2;c.shadowColor=p.mid;c.shadowBlur=8;c.beginPath();for(let i=0;i<this.wave.length;i+=4){const x=i/(this.wave.length-1)*w,y=cy+(this.wave[i]-128)/128*h*.43;i?c.lineTo(x,y):c.moveTo(x,y)}c.stroke();c.shadowBlur=0;return}
 if(this.mode==='radial'){c.save();c.translate(cx,cy);c.rotate(this.phase*.18);for(let i=0;i<128;i++){const a=i/128*Math.PI*2,v=this.bars[i%64],r0=r*(.65+this.energy*.25);c.strokeStyle=v>.7?p.high:v>.35?p.mid:p.low;c.lineWidth=Math.max(2,w/320);c.beginPath();c.moveTo(Math.cos(a)*r0,Math.sin(a)*r0);c.lineTo(Math.cos(a)*(r0+r*v),Math.sin(a)*(r0+r*v));c.stroke()}c.strokeStyle=p.mid;c.beginPath();c.arc(0,0,r*.5,0,Math.PI*2);c.stroke();c.restore();return}
 if(this.mode==='waterfall'){const x=this.waterCtx;x.globalCompositeOperation='copy';x.drawImage(this.waterfall,0,-1);x.globalCompositeOperation='source-over';for(let i=0;i<64;i++){const v=this.bars[i];x.fillStyle=p.bg;x.fillRect(i*5,179,5,1);x.globalAlpha=v;x.fillStyle=v>.75?p.high:v>.35?p.mid:p.low;x.fillRect(i*5,179,5,1);x.globalAlpha=1}c.imageSmoothingEnabled=false;c.drawImage(this.waterfall,0,0,w,h);c.imageSmoothingEnabled=true;return}
 if(this.mode==='plasma'){const t=this.trailCtx;const tw=640,th=360;t.save();t.globalAlpha=.96;t.translate(tw/2,th/2);t.rotate(.005+this.energy*.02);t.scale(1.012+this.energy*.024,1.012+this.energy*.024);t.drawImage(this.trail,-tw/2,-th/2);t.restore();t.fillStyle=p.bg+'12';t.fillRect(0,0,tw,th);t.globalCompositeOperation='lighter';t.lineWidth=2;t.strokeStyle=p.mid;t.beginPath();for(let i=0;i<=360;i++){const a=i*Math.PI/180,v=this.bars[Math.floor(i/360*63)],rr=55+v*65+this.energy*42,x=tw/2+Math.sin(a*3+this.phase)*rr+Math.cos(a*2)*35,y=th/2+Math.cos(a*2-this.phase)*rr;i?t.lineTo(x,y):t.moveTo(x,y)}t.closePath();t.stroke();t.strokeStyle=p.low;t.beginPath();for(let i=0;i<512;i+=2){const a=i/512*Math.PI*2,rr=95+(this.wave[i]-128)*.85;t.lineTo(tw/2+Math.cos(a+this.phase)*rr,th/2+Math.sin(a+this.phase)*rr)}t.stroke();t.globalCompositeOperation='source-over';c.drawImage(this.trail,0,0,w,h);return}
 // Deterministic moving starfield; speed and brightness follow audio energy.
 c.save();c.translate(cx,cy);for(let i=0;i<180;i++){const a=i*2.399963,depth=1-((i*.618033+this.phase*.12)%1),d=(1-depth)*Math.min(w,h)*.72;const x=Math.cos(a)*d,y=Math.sin(a)*d;const v=this.bars[i%64];c.strokeStyle=v>.5?p.high:p.mid;c.globalAlpha=.18+.7*(1-depth);c.lineWidth=1+v*2;c.beginPath();c.moveTo(x*.93,y*.93);c.lineTo(x*(1+dt*this.energy*3),y*(1+dt*this.energy*3));c.stroke()}c.restore();
 }
}
