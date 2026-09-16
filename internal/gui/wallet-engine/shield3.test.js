import test from 'node:test';
import assert from 'node:assert/strict';
import {validateShield3Amount, SHIELD3_MAX_SEND_WEI} from './shield3.js';
test('Shield3 amount cap uses exact smallest units',()=>{
 assert.equal(validateShield3Amount('5000000'),SHIELD3_MAX_SEND_WEI);
 assert.equal(validateShield3Amount('4999999.999999999999999999'),SHIELD3_MAX_SEND_WEI-1n);
 assert.equal(validateShield3Amount('0.000000000000000001'),1n);
 for(const amount of ['0','-1','5000000.000000000000000001','1e6','NaN','0.0000000000000000001'])assert.throws(()=>validateShield3Amount(amount));
});
