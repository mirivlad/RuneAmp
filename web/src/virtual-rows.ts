/** Fixed-height windowed list. Keeps the scroll extent without creating every row. */
export class VirtualRows {
 private count=0;
 private make:(i:number)=>HTMLElement=()=>document.createElement('div');
 private frame=0;
 private first=-1;
 private last=-1;
 constructor(private root:HTMLElement,private height:number){
  root.classList.add('virtual-rows');
  root.addEventListener('scroll',()=>this.request(),{passive:true});
 }
 set(count:number,make:(i:number)=>HTMLElement){this.count=count;this.make=make;this.refresh()}
 refresh(){this.first=-1;this.last=-1;this.draw()}
 scrollTo(i:number,center=false){if(i<0||i>=this.count)return;this.root.scrollTop=i*this.height-(center?Math.max(0,(this.root.clientHeight-this.height)/2):0);this.refresh()}
 private request(){if(!this.frame)this.frame=requestAnimationFrame(()=>{this.frame=0;this.draw()})}
 private draw(){
  const height=this.height,root=this.root;
  const first=Math.min(this.count,Math.max(0,Math.floor(Math.min(root.scrollTop,Math.max(0,this.count*height-(root.clientHeight||520)))/height)-8));
  const last=Math.min(this.count,first+Math.ceil((root.clientHeight||520)/height)+24);
  if(first===this.first&&last===this.last)return;
  this.first=first;this.last=last;
  const fragment=document.createDocumentFragment();
  const spacer=(n:number)=>{const e=document.createElement('div');e.setAttribute('aria-hidden','true');e.style.cssText='height:'+n+'px;flex:0 0 '+n+'px;pointer-events:none';return e};
  if(first)fragment.append(spacer(first*height));
  for(let i=first;i<last;i++){const r=this.make(i);r.dataset.index=String(i);r.style.height=height+'px';r.style.minHeight=height+'px';r.style.maxHeight=height+'px';r.style.flex='0 0 '+height+'px';r.style.overflow='hidden';fragment.append(r)}
  if(last<this.count)fragment.append(spacer((this.count-last)*height));
  root.replaceChildren(fragment);
 }
}
