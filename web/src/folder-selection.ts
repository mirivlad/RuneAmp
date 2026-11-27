type IndexedTrack = {id:string; source:string; path:string};
type Selection = {source:string; entry:{path:string; directory:boolean}};
/** Match indexed tracks by source and whole path components, at any depth. */
export function selectedFolderTracks<T extends IndexedTrack>(library:readonly T[], selection:readonly Selection[]):T[]{
 const files=new Set<string>(),folders=new Set<string>();
 const key=(source:string,path:string)=>source+'\0'+path;
 for(const {source,entry} of selection){
  (entry.directory?folders:files).add(key(source,entry.path));
 }
 const seen=new Set<string>();
 return library.filter(t=>{
  if(seen.has(t.id))return false;
  let match=files.has(key(t.source,t.path))||folders.has(key(t.source,'.'));
  for(let i=t.path.lastIndexOf('/');!match&&i>=0;i=t.path.lastIndexOf('/',i-1)){
   match=folders.has(key(t.source,t.path.slice(0,i)));
   if(i===0)break;
  }
  if(match)seen.add(t.id);
  return match;
 });
}
