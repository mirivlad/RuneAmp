import {unzipSync} from 'fflate';
type Skin={name:string,main:ImageBitmap,buttons?:ImageBitmap,title?:ImageBitmap,playlist?:ImageBitmap,volume?:ImageBitmap,posbar?:ImageBitmap,shufrep?:ImageBitmap,numbers?:ImageBitmap,text?:ImageBitmap,playpaus?:ImageBitmap,balance?:ImageBitmap,mono?:ImageBitmap,colors:string[],fg:string,bg:string,selected:string,current:string};
const MAX=8*1024*1024;
const supported=new Set(['main.bmp','cbuttons.bmp','titlebar.bmp','pledit.bmp','pledit.txt','viscolor.txt','volume.bmp','posbar.bmp','shufrep.bmp','numbers.bmp','nums_ex.bmp','text.bmp','playpaus.bmp','balance.bmp','monoster.bmp']);
export async function decodeSkin(file:File):Promise<Skin>{
 if(file.size>MAX||file.size<22)throw Error('Архив должен быть не больше 8 МиБ');
 let total=0;const archive=unzipSync(new Uint8Array(await file.arrayBuffer()),{filter:f=>{const n=f.name.split(/[\\/]/).pop()?.toLowerCase()??'';if(!supported.has(n))return false;total+=f.originalSize;if(f.originalSize>2*1024*1024||total>MAX)throw Error('Слишком большой распакованный скин');return true}});
 const files=new Map<string,Uint8Array>();for(const [path,data] of Object.entries(archive)){const name=path.split(/[\\/]/).pop()!.toLowerCase();if(files.has(name))throw Error('Повторяющийся файл скина');files.set(name,data)}
 async function bitmap(n:string){const data=files.get(n);if(!data)return undefined;
 const view=new DataView(data.buffer,data.byteOffset,data.byteLength);if(data.length<26||view.getUint16(0,true)!==0x4d42)throw Error('Некорректный BMP: '+n);const header=view.getUint32(14,true);const w=header===12?view.getUint16(18,true):view.getInt32(18,true),h=header===12?view.getUint16(20,true):Math.abs(view.getInt32(22,true));if(w<1||h<1||w>2048||h>2048||w*h>2000000)throw Error('Слишком большой BMP');
 const image=await createImageBitmap(new Blob([new Uint8Array(data)],{type:'image/bmp'}));return image}
 const main=await bitmap('main.bmp');if(!main||main.width<275||main.height<116){main?.close();throw Error('Нет корректного MAIN.BMP (275×116)')}
 const skin:Skin={name:file.name.replace(/\.(wsz|zip)$/i,''),main,colors:[],fg:'#d4e7cb',bg:'#131b13',selected:'#324331',current:'#ffffff'};
 try{skin.buttons=await bitmap('cbuttons.bmp');skin.title=await bitmap('titlebar.bmp');skin.playlist=await bitmap('pledit.bmp');skin.volume=await bitmap('volume.bmp');skin.posbar=await bitmap('posbar.bmp');skin.shufrep=await bitmap('shufrep.bmp');skin.numbers=await bitmap('nums_ex.bmp')??await bitmap('numbers.bmp');skin.text=await bitmap('text.bmp');skin.playpaus=await bitmap('playpaus.bmp');skin.balance=await bitmap('balance.bmp');skin.mono=await bitmap('monoster.bmp');skin.colors=new TextDecoder().decode(files.get('viscolor.txt')).split(/\r?\n/).flatMap(line=>{const m=line.match(/^\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/);return m?[('rgb('+m.slice(1).map(v=>Math.min(255,Number(v))).join(',')+')')]:[]}).slice(0,24);const text=new TextDecoder().decode(files.get('pledit.txt'));const color=(key:string)=>text.match(new RegExp('^\\s*'+key+'\\s*=\\s*(#[0-9a-f]{6})','im'))?.[1];skin.fg=color('Normal')??skin.fg;skin.bg=color('NormalBG')??skin.bg;skin.selected=color('SelectedBG')??skin.selected;skin.current=color('Current')??skin.current;return skin}catch(e){disposeSkin(skin);throw e}
}
export function disposeSkin(s:Skin){for(const image of [s.main,s.buttons,s.title,s.playlist,s.volume,s.posbar,s.shufrep,s.numbers,s.text,s.playpaus,s.balance,s.mono])image?.close()}
type SkinState={name:string,time:string,playing:boolean,paused:boolean,volume:number,position:number,seekable:boolean,shuffle:boolean,repeat:string,playlist:boolean,levels?:Uint8Array}
function sprite(c:CanvasRenderingContext2D,img:ImageBitmap|undefined,sx:number,sy:number,w:number,h:number,x:number,y:number){if(!img||sx+w>img.width||sy+h>img.height)return false;c.drawImage(img,sx,sy,w,h,x,y,w,h);return true}
function text(c:CanvasRenderingContext2D,s:Skin,value:string,x:number,y:number,max:number){
 const alphabet='abcdefghijklmnopqrstuvwxyz"@  ',symbols="0123456789….:()-'!_+\\/[]^&%,=\$#";
 const chars=Array.from(value.toLowerCase());
 if(s.text&&chars.every(v=>alphabet.includes(v)||symbols.includes(v)||v==='?'||v==='*')){
  for(const [i,v] of chars.slice(0,Math.floor(max/5)).entries()){let row=0,col=alphabet.indexOf(v);if(v===' ')col=30;else if(col<0){row=1;col=symbols.indexOf(v);if(col<0){row=2;col=v==='?'?3:4}}sprite(c,s.text,col*5,row*6,5,6,x+i*5,y)}
 }else{c.fillStyle=s.fg;c.font='7px monospace';c.save();c.beginPath();c.rect(x,y,max,8);c.clip();c.fillText(value,x,y+7);c.restore()}
}
function fallback(c:CanvasRenderingContext2D,s:Skin,x:number,y:number,w:number,h:number,label:string){c.fillStyle=s.bg;c.fillRect(x,y,w,h);c.strokeStyle=s.fg;c.strokeRect(x+.5,y+.5,w-1,h-1);c.fillStyle=s.fg;c.font='8px monospace';c.textAlign='center';c.fillText(label,x+w/2,y+h/2+3);c.textAlign='left'}
export function drawSkin(canvas:HTMLCanvasElement,s:Skin,state:SkinState){
 const c=canvas.getContext('2d')!;c.imageSmoothingEnabled=false;
 c.drawImage(s.main,0,0,275,116,0,0,275,116);
 // Classic WSZ sprite layout: https://winampskins.neocities.org/main
 sprite(c,s.title,27,0,275,14,0,0);
 for(const [sx,x,w,label] of [[0,16,23,'|<'],[23,39,23,'>'],[46,62,23,'II'],[69,85,23,'[]'],[92,108,22,'>|']] as const)if(!sprite(c,s.buttons,sx,0,w,18,x,88))fallback(c,s,x,88,w,18,label);
 if(!sprite(c,s.buttons,114,0,22,16,136,89))fallback(c,s,136,89,22,16,'^');
 sprite(c,s.playpaus,state.playing?0:state.paused?9:18,0,9,9,24,28);
 const digits=state.time.replace(/[^0-9]/g,'').padStart(4,'0').slice(-4);
 if(s.numbers&&s.numbers.width>=90&&s.numbers.height>=13){for(const [i,x] of [48,60,78,90].entries())sprite(c,s.numbers,Number(digits[i])*9,0,9,13,x,26)}
 else{c.fillStyle=s.fg;c.font='bold 14px monospace';c.fillText(state.time,40,39)}
 text(c,s,state.name,112,27,151);
 if(!sprite(c,s.volume,0,Math.round(state.volume*27)*15,68,13,107,57))fallback(c,s,107,57,68,13,'VOL');
 if(!sprite(c,s.volume,15,422,14,11,107+Math.round(state.volume*54),58)){c.fillStyle=s.fg;c.fillRect(107+Math.round(state.volume*62),58,5,11)}
 sprite(c,s.balance,9,0,38,13,177,57);sprite(c,s.balance,15,422,14,11,189,58);
 if(!sprite(c,s.posbar,0,0,248,10,16,72))fallback(c,s,16,72,248,10,'');
 if(state.seekable&&!sprite(c,s.posbar,248,0,29,10,16+Math.round(state.position*219),72)){c.fillStyle=s.fg;c.fillRect(16+Math.round(state.position*219),73,12,8)}
 if(!sprite(c,s.shufrep,28,state.shuffle?30:0,47,15,164,89))fallback(c,s,164,89,47,15,'SHUF');
 if(!sprite(c,s.shufrep,0,state.repeat==='off'?0:30,28,15,210,89))fallback(c,s,210,89,28,15,'REP');
 sprite(c,s.shufrep,0,61,23,12,219,58);
 if(!sprite(c,s.shufrep,23,state.playlist?73:61,23,12,242,58))fallback(c,s,242,58,23,12,'PL');
 const palette=s.colors.length>=18?s.colors.slice(2,18):['#235c28','#4e9134','#89ca45','#d3d45a','#e6a74a','#ff663b'];
 const levels=state.levels;if(levels){for(let i=0;i<19;i++){const h=Math.round(levels[i*3]/255*15);for(let j=0;j<h;j++){c.fillStyle=palette[Math.min(palette.length-1,Math.floor(j*palette.length/15))];c.fillRect(24+i*4,58-j,3,1)}}}
}
export function drawPlaylistHeader(canvas:HTMLCanvasElement,s:Skin):boolean{
 if(!s.playlist||s.playlist.width<178||s.playlist.height<20)return false;
 const c=canvas.getContext('2d')!;c.imageSmoothingEnabled=false;
 for(let x=0;x<275;x+=25)sprite(c,s.playlist,127,0,25,20,x,0);
 sprite(c,s.playlist,0,0,25,20,0,0);sprite(c,s.playlist,26,0,100,20,87,0);sprite(c,s.playlist,153,0,25,20,250,0);return true
}
function db():Promise<IDBDatabase>{return new Promise((resolve,reject)=>{const q=indexedDB.open('runeamp-skins',1);q.onupgradeneeded=()=>q.result.createObjectStore('skins');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error)})}
export async function savedSkins():Promise<File[]>{const d=await db();try{return await new Promise((resolve,reject)=>{const q=d.transaction('skins','readonly').objectStore('skins').getAll();q.onsuccess=()=>resolve(q.result as File[]);q.onerror=()=>reject(q.error)})}finally{d.close()}}
export async function storeSkin(file:File,remove=false){const d=await db();try{await new Promise<void>((resolve,reject)=>{const tx=d.transaction('skins','readwrite');if(remove)tx.objectStore('skins').delete(file.name);else tx.objectStore('skins').put(file,file.name);tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error)})}finally{d.close()}}
