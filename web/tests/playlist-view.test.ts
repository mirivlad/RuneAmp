import {test} from 'node:test';
import assert from 'node:assert/strict';
import {playlistURL,playlistKey,coverColors} from '../src/playlist-view.ts';
test('playlist link encodes special characters and keeps scopes distinct',()=>{for(const scope of ['public','personal'] as const){const key={name:'ZX & Amiga / музыка #1?',scope};assert.deepEqual(playlistKey(new URL(playlistURL(key),'https://example.com').search),key)}assert.equal(playlistKey('?scope=other&name=x'),undefined);assert.equal(playlistKey('?scope=public'),undefined)});
test('cover colors deterministic and bounded',()=>{const key={name:'ZX',scope:'public' as const};assert.deepEqual(coverColors(key),coverColors(key));assert(coverColors(key).every(n=>n>=0&&n<360));assert.notDeepEqual(coverColors(key),coverColors({...key,scope:'personal'}))});
