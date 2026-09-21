import test from 'node:test';
import assert from 'node:assert/strict';
import {ethers} from 'ethers';
import {shieldedMimcHash} from './vendor/shielded.js';
import {validateShield2Migration} from './shield3-migrate.js';
test('legacy migration cannot create private change or redirect the withdrawal',()=>{
 const address='0x1111111111111111111111111111111111111111',note={noteValueWei:'10',nullifier:ethers.toBeHex(7,32),merkleRoot:ethers.toBeHex(8,32)};
 const random=[1n,2n,3n,4n];
 const outputs=random.map((r,i)=>[ethers.toBeHex(shieldedMimcHash([2001n,i===0?BigInt(address):0n,1n,0n,r]),32),ethers.ZeroHash,'0x','0x','0x','0x']);
 const envelope=['0x02',[[note.nullifier,note.merkleRoot,'0x','0x']],outputs,ethers.ZeroHash,'0x',address,'0x0a','0x',random.map(r=>ethers.toBeHex(r,32))];
 const response={shieldedVersion:2,outputOpenings:[],transaction:{chainId:8979,to:'0x00000000000000000000000000000000000000f7',value:0,gas:3000000,nonce:0,gasFeeCap:1,gasTipCap:1,accessList:[]}};
 const update=()=>response.transaction.data=ethers.concat([ethers.toUtf8Bytes('TKMSHIELD1'),ethers.encodeRlp(envelope)]);update();
 const expected={address,note,nonce:0,gasPrice:1n};
 assert.equal(validateShield2Migration(response,expected),response.transaction);
 outputs[2][0]=ethers.toBeHex(shieldedMimcHash([2001n,0n,1n,1n,random[2]]),32);update();
 assert.throws(()=>validateShield2Migration(response,expected),/private change/);
 outputs[2][0]=ethers.toBeHex(shieldedMimcHash([2001n,0n,1n,0n,random[2]]),32);envelope[5]='0x2222222222222222222222222222222222222222';update();
 assert.throws(()=>validateShield2Migration(response,expected),/recipient/);
});
