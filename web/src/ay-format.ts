// Numeric IDs are part of the freestanding WASM replay ABI.
export const ayFormats={pt1:1,pt2:2,pt3:3,stp:4,asc:5,as0:6,psc:7,sqt:8,stc:9,psg:10,ym:11,vtx:12} as const;
export type AYFormat=keyof typeof ayFormats;
export function formatOf(name:string):AYFormat|undefined{const ext=name.split('.').at(-1)?.toLowerCase()??'';return Object.prototype.hasOwnProperty.call(ayFormats,ext)?ext as AYFormat:undefined}
export function maxAYSize(format:AYFormat){return ayFormats[format]>=10?2*1024*1024:65536}
