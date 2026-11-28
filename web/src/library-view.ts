export type LibraryTrack={id:string;name:string;path:string;source:string;format:string;mode:string;size:number};
export type LibraryFilter={query:string;source:string;folder:string;recursive:boolean;format:string;sort:string;ids?:readonly string[]};
const collator=new Intl.Collator(undefined,{numeric:true,sensitivity:'base'});
export function filterLibrary<T extends LibraryTrack>(tracks:readonly T[],f:LibraryFilter):T[]{
 const ids=f.ids?new Set(f.ids):undefined,q=f.query.trim().toLocaleLowerCase();
 const out=tracks.filter(t=>{
  const parent=t.path.includes('/')?t.path.slice(0,t.path.lastIndexOf('/')):'';
  return (!ids||ids.has(t.id))&&(f.source==='all'||t.source===f.source)&&
   (f.recursive?(!f.folder||parent===f.folder||parent.startsWith(f.folder+'/')):parent===f.folder)&&
   (f.format==='all'||t.format===f.format)&&(!q||(t.name+' '+t.path).toLocaleLowerCase().includes(q));
 });
 const cmp=(a:T,b:T)=>collator.compare(a.name,b.name)||collator.compare(a.path,b.path)||collator.compare(a.id,b.id);
 if(f.sort==='name')out.sort(cmp);
 else if(f.sort==='name-desc')out.sort((a,b)=>cmp(b,a));
 else if(f.sort==='size')out.sort((a,b)=>b.size-a.size||cmp(a,b));
 else if(f.sort==='format')out.sort((a,b)=>collator.compare(a.format,b.format)||cmp(a,b));
 else if(f.sort==='recent'&&f.ids){const rank=new Map(f.ids.map((id,i)=>[id,i]));out.sort((a,b)=>rank.get(a.id)!-rank.get(b.id)!)}
 return out;
}
export function ancestors(path:string):string[]{const result:string[]=[];let p='';for(const part of path.split('/').filter(Boolean)){p=p?p+'/'+part:part;result.push(p)}return result}
