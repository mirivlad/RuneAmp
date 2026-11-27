const tag=(b:Uint8Array,p:number,s:string)=>p>=0&&p+s.length<=b.length&&[...s].every((c,i)=>b[p+i]===c.charCodeAt(0));
const le16=(b:Uint8Array,p:number)=>b[p]+256*b[p+1];
export function splitTurboSound(b:Uint8Array):[Uint8Array,Uint8Array]|undefined{
 if(tag(b,b.length-4,'03TS'))throw Error('Три AY-чипа пока не поддерживаются.');
 if(tag(b,b.length-4,'02TS')){
  const p=b.length-16;
  if(!tag(b,p,'PT3!')||!tag(b,p+6,'PT3!'))throw Error('Некорректный контейнер TurboSound.');
  const a=le16(b,p+4),c=le16(b,p+10);
  if(a<203||c<203||a+c!==p)throw Error('Некорректные размеры TurboSound.');
  return [b.subarray(0,a),b.subarray(a,a+c)];
 }
 // Vortex Tracker native TurboSound stores both chips in one PT3.
 // The second position list is mirrored about marker * 3 - 3.
 if(b.length<203||b[98]===32)return;
 const end=b.subarray(201,Math.min(457,b.length)).indexOf(255),marker=b[98];
 if(b.length>65536||marker<1||marker>86||end<=0||b[100]===0)throw Error('Некорректный шестиканальный PT3.');
 const first=Uint8Array.from(b),second=Uint8Array.from(b),ptr=le16(b,103);
 first[98]=second[98]=32;
 for(let i=0;i<end;i++){
  const a=b[201+i],c=marker*3-3-a;
  if(a%3||c<0||c>254||c%3||ptr+2*Math.max(a,c)+6>b.length)throw Error('Некорректный указатель TurboSound.');
  second[201+i]=c;
 }
 return [first,second];
}
export function validSinglePT3(b:Uint8Array){return b.length>=203&&b.length<=65536&&b[98]===32&&b[100]!==0&&b.subarray(201,Math.min(457,b.length)).indexOf(255)>0}
