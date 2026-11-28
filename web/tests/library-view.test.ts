import {test} from 'node:test';
import assert from 'node:assert/strict';
import {filterLibrary,ancestors} from '../src/library-view.ts';
const tracks=[{id:'1',name:'song10',path:'ZX/A/song10',source:'music',format:'ts',mode:'ay',size:10},{id:'2',name:'song2',path:'ZX/A/sub/song2',source:'music',format:'pt3',mode:'ay',size:20},{id:'3',name:'song1',path:'ZX/AB/song1',source:'music',format:'ts',mode:'ay',size:1},{id:'4',name:'song4',path:'ZX/A/song4',source:'uploads',format:'ts',mode:'ay',size:4}];
const f={query:'',source:'all',folder:'ZX/A',recursive:true,format:'all',sort:'original'};
test('folder and source boundaries, recursive and direct',()=>{assert.deepEqual(filterLibrary(tracks,f).map(t=>t.id),['1','2','4']);assert.deepEqual(filterLibrary(tracks,{...f,recursive:false,source:'music'}).map(t=>t.id),['1'])});
test('format, search, numeric title sort and size',()=>{assert.deepEqual(filterLibrary(tracks,{...f,format:'ts',sort:'name',query:'song'}).map(t=>t.id),['4','1']);assert.deepEqual(filterLibrary(tracks,{...f,sort:'size'}).map(t=>t.id),['2','1','4'])});
test('recent order and ancestor paths',()=>{assert.deepEqual(filterLibrary(tracks,{...f,ids:['4','1'],sort:'recent'}).map(t=>t.id),['4','1']);assert.deepEqual(ancestors('ZX/A/sub'),['ZX','ZX/A','ZX/A/sub'])});
