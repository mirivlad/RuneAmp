import {test} from 'node:test';
import assert from 'node:assert/strict';
import {selectedFolderTracks} from '../src/folder-selection.ts';
const tracks=[
 {id:'a',source:'music',path:'ZX/root.pt3'},
 {id:'b',source:'music',path:'ZX/a/b/c/d/e/deep.pt3'},
 {id:'c',source:'music',path:'ZX2/other.pt3'},
 {id:'d',source:'uploads',path:'ZX/root.pt3'},
];
const pick=(path:string,directory=true,source='music')=>({source,entry:{path,directory}});
test('recursive folder selection respects source and path boundaries',()=>assert.deepEqual(selectedFolderTracks(tracks,[pick('ZX')]).map(t=>t.id),['a','b']));
test('mixed overlapping selections deduplicate',()=>assert.deepEqual(selectedFolderTracks([...tracks,tracks[0]],[pick('ZX'),pick('ZX/a'),pick('ZX/root.pt3',false)]).map(t=>t.id),['a','b']));
test('direct files and root directory',()=>{assert.deepEqual(selectedFolderTracks(tracks,[pick('ZX/root.pt3',false,'uploads')]).map(t=>t.id),['d']);assert.equal(selectedFolderTracks(tracks,[pick('.')]).length,3)});
