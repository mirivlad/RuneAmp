// Themes are declarative data, not executable CSS. No layout or player selectors are accepted.
export type ThemeManifest={version:1,name:string,dark:Record<string,string>,light:Record<string,string>,radius:number,typography:'system'|'mono'|'serif',artwork:'industrial'|'geometry'|'records',images?:Record<string,string>};
export type ServiceTheme={id:string,manifest?:ThemeManifest,images?:Record<string,string>};
const keys=['bg','panel','line','text','muted','accent','primary','hover','buttonText','heroA','heroB','personal','zx','ts','tracker','audio'];
const slots=['hero','personal','zx','ts','tracker','audio','cover'];
let current:ServiceTheme={id:'default'};
const safe=()=>location.pathname==='/admin'||new URLSearchParams(location.search).get('safe-theme')==='1';
export function renderServiceTheme(){
 const root=document.getElementById('service')!;
 for(const key of keys)root.style.removeProperty('--theme-'+key);
 for(const slot of slots)root.style.removeProperty('--theme-image-'+slot);
 root.style.removeProperty('--theme-radius');
 root.removeAttribute('data-service-theme');root.removeAttribute('data-theme-type');root.removeAttribute('data-theme-art');
 if(safe())return;
 root.dataset.serviceTheme='default';root.dataset.themeType='mono';root.dataset.themeArt='industrial';
 const m=current.manifest;if(!m||m.version!==1)return;
 const palette=document.body.classList.contains('light')?m.light:m.dark;
 if(!palette||!keys.every(k=>/^#[0-9a-f]{6}$/i.test(palette[k]??'')))return;
 root.dataset.serviceTheme='custom';
 root.dataset.themeType=['system','mono','serif'].includes(m.typography)?m.typography:'system';
 root.dataset.themeArt=['industrial','geometry','records'].includes(m.artwork)?m.artwork:'industrial';
 for(const k of keys)root.style.setProperty('--theme-'+k,palette[k]);
 if(Number.isInteger(m.radius)&&m.radius>=0&&m.radius<=16)root.style.setProperty('--theme-radius',m.radius+'px');
 for(const s of slots){const v=current.images?.[s];if(v&&/^data:image\/(png|jpeg);base64,[A-Za-z0-9+/]+={0,2}$/.test(v))root.style.setProperty('--theme-image-'+s,'url("'+v+'")')}
}
export function setServiceTheme(t:ServiceTheme){current=t?.id?t:{id:'default'};renderServiceTheme()}
export async function loadServiceTheme(){try{const r=await fetch('/api/theme',{cache:'no-store'});if(r.ok)setServiceTheme(await r.json());else setServiceTheme({id:'default'})}catch{setServiceTheme({id:'default'})}}
