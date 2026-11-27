import {test} from 'node:test';
import assert from 'node:assert/strict';
import {splitTurboSound,validSinglePT3} from '../src/turbosound.ts';
function moduleBytes(n:number){const b=Buffer.alloc(n);b[98]=32;b[100]=1;b[202]=255;return b}
function pair(){const a=moduleBytes(300),b=moduleBytes(400),tail=Buffer.alloc(16);tail.write('PT3!');tail.writeUInt16LE(a.length,4);tail.write('PT3!',6);tail.writeUInt16LE(b.length,10);tail.write('02TS',12);return Buffer.concat([a,b,tail])}
test('02TS splits two modules without changing bytes',()=>{const b=pair(),p=splitTurboSound(b)!;assert.deepEqual(p.map(x=>x.length),[300,400]);assert(p.every(validSinglePT3));assert.deepEqual(p[1],b.subarray(300,700))});
test('rejects wrong lengths and signatures',()=>{const b=pair();b[b.length-12]=0;assert.throws(()=>splitTurboSound(b));const c=pair();c[c.length-16]=0;assert.throws(()=>splitTurboSound(c))});
test('ordinary PT3 and unsupported three-chip format',()=>{assert.equal(splitTurboSound(moduleBytes(300)),undefined);assert.throws(()=>splitTurboSound(Buffer.from('03TS')))});
test('native TS mirrors positions without mutating source bytes',()=>{
 const b=moduleBytes(600);b[98]=48;b.writeUInt16LE(220,103);b[201]=141;b[202]=138;b[203]=255;const before=Buffer.from(b);
 const [a,c]=splitTurboSound(b)!;assert.deepEqual([...a.subarray(201,204)],[141,138,255]);assert.deepEqual([...c.subarray(201,204)],[0,3,255]);assert.equal(a[98],32);assert.equal(c[98],32);assert.deepEqual(b,before);
 b[201]=142;assert.throws(()=>splitTurboSound(b));
});
