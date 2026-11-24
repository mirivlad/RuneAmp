import {unzipSync} from 'fflate';
type Skin={name:string,main:ImageBitmap,buttons?:ImageBitmap,title?:ImageBitmap,playlist?:ImageBitmap,fg:string,bg:string};
const MAX=8*1024*1024;
const supported=new Set(['main.bmp','cbuttons.bmp','titlebar.bmp','pledit.bmp','pledit.txt','viscolor.txt']);
export async function decodeSkin(file:File):Promise<Skin>{
 if(file.size>MAX||file.size<22)throw Error('Архив должен быть не больше 8 МиБ');
 let total=0;const archive=unzipSync(new Uint8Array(await file.arrayBuffer()),{filter:f=>{const n=f.name.split(/[\\/]/).pop()?.toLowerCase()??'';if(!supported.has(n))return false;total+=f.originalSize;if(f.originalSize>2*1024*1024||total>MAX)throw Error('Слишком большой распакованный скин');return true}});
 const files=new Map<string,Uint8Array>();for(const [path,data] of Object.entries(archive)){const name=path.split(/[\\/]/).pop()!.toLowerCase();if(files.has(name))throw Error('Повторяющийся файл скина');files.set(name,data)}
 async function bitmap(n:string){const data=files.get(n);if(!data)return undefined;
 const view=new DataView(data.buffer,data.byteOffset,data.byteLength);if(data.length<26||view.getUint16(0,true)!==0x4d42)throw Error('Некорректный BMP: '+n);const header=view.getUint32(14,true);const w=header===12?view.getUint16(18,true):view.getInt32(18,true),h=header===12?view.getUint16(20,true):Math.abs(view.getInt32(22,true));if(w<1||h<1||w>2048||h>2048||w*h>2000000)throw Error('Слишком большой BMP');
 const image=await createImageBitmap(new Blob([new Uint8Array(data)],{type:'image/bmp'}));return image}
 const main=await bitmap('main.bmp');if(!main||main.width<275||main.height<116){main?.close();throw Error('Нет корректного MAIN.BMP (275×116)')}
 const skin:Skin={name:file.name.replace(/\.(wsz|zip)$/i,''),main,fg:'#d4e7cb',bg:'#131b13'};
 try{skin.buttons=await bitmap('cbuttons.bmp');skin.title=await bitmap('titlebar.bmp');skin.playlist=await bitmap('pledit.bmp');const text=new TextDecoder().decode(files.get('pledit.txt'));const color=(key:string)=>text.match(new RegExp('^\\s*'+key+'\\s*=\\s*(#[0-9a-f]{6})','im'))?.[1];skin.fg=color('Normal')??skin.fg;skin.bg=color('NormalBG')??skin.bg;return skin}catch(e){disposeSkin(skin);throw e}
}
export function disposeSkin(s:Skin){s.main.close();s.buttons?.close();s.title?.close();s.playlist?.close()}
export function drawSkin(canvas:HTMLCanvasElement,s:Skin,name:string,time:string,playing:boolean,levels?:Uint8Array){const c=canvas.getContext('2d')!;c.imageSmoothingEnabled=false;c.drawImage(s.main,0,0,275,116,0,0,275,116);
 if(s.title&&s.title.width>=275&&s.title.height>=15)c.drawImage(s.title,0,0,275,14,0,0,275,14);
 if(s.buttons&&s.buttons.width>=114&&s.buttons.height>=18){const y=0;for(const [sx,x,w] of [[0,16,23],[23,39,23],[46,62,23],[69,85,23],[92,108,22]])c.drawImage(s.buttons,sx,y,w,18,x,88,w,18);if(playing&&s.buttons.height>=36)c.drawImage(s.buttons,23,18,23,18,39,88,23,18)}
 c.fillStyle=s.fg;c.font='9px monospace';c.save();c.beginPath();c.rect(112,23,154,13);c.clip();c.fillText(name,112,32);c.restore();c.font='bold 16px monospace';c.fillText(time,39,43);if(levels){for(let i=0;i<18;i++){const h=levels[i*3]/255*14;c.fillStyle=i%3===0?'#ffbd53':s.fg;c.fillRect(25+i*4,73-h,3,h)}}}
function db():Promise<IDBDatabase>{return new Promise((resolve,reject)=>{const q=indexedDB.open('runeamp-skins',1);q.onupgradeneeded=()=>q.result.createObjectStore('skins');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error)})}
export async function savedSkins():Promise<File[]>{const d=await db();try{return await new Promise((resolve,reject)=>{const q=d.transaction('skins','readonly').objectStore('skins').getAll();q.onsuccess=()=>resolve(q.result as File[]);q.onerror=()=>reject(q.error)})}finally{d.close()}}
export async function storeSkin(file:File,remove=false){const d=await db();try{await new Promise<void>((resolve,reject)=>{const tx=d.transaction('skins','readwrite');if(remove)tx.objectStore('skins').delete(file.name);else tx.objectStore('skins').put(file,file.name);tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error)})}finally{d.close()}}
