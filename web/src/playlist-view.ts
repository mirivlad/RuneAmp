export type PlaylistKey={name:string,scope?:'public'|'personal'};
export function playlistURL(p:PlaylistKey){return '/playlists?'+new URLSearchParams({scope:p.scope??'public',name:p.name}).toString()}
export function playlistKey(search:string):PlaylistKey|undefined{const q=new URLSearchParams(search),name=q.get('name'),scope=q.get('scope');if(!name||(scope!=='public'&&scope!=='personal'))return;return {name,scope}}
export function coverColors(p:PlaylistKey):[number,number]{let h=2166136261;for(const c of (p.scope??'public')+'\0'+p.name){h=Math.imul(h^c.charCodeAt(0),16777619)}const hue=(h>>>0)%360;return [hue,(hue+42)%360]}
export function playlistCover(p:PlaylistKey):HTMLElement{
 const cover=document.createElement('div');cover.className='playlist-cover';cover.setAttribute('aria-hidden','true');
 const [a,b]=coverColors(p);cover.style.setProperty('--cover-a','hsl('+a+' 44% 32%)');cover.style.setProperty('--cover-b','hsl('+b+' 52% 18%)');
 const mark=document.createElement('span');mark.className='cover-mark';mark.textContent='♫';cover.append(mark);
 const title=document.createElement('span');title.className='cover-title';title.textContent=p.name;cover.append(title);
 return cover;
}
