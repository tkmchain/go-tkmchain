import test from 'node:test';
import assert from 'node:assert/strict';
import { ethers } from 'ethers';
import { planPayment, planRecipientPaidPayment, validateResponse, sendTKM } from './shield2-send.js';
import { shieldedMimcHash, deriveShieldedIdentity } from './vendor/shielded.js';
import { createPQKeystore } from './vendor/pq.js';
import { createServer } from 'node:http';
import { once } from 'node:events';

const note = value => ({ status: 'available', version: 2, assetId: '1', noteValueWei: String(value) });
test('splits notes with gas sponsorship and preserves requested total', () => {
    const parts = planPayment([note(100), note(100)], 150n, 10n, 0n, true);
    assert.deepEqual(parts.map(p => [p.value, p.sponsor]), [[90n, 10n], [60n, 10n]]);
});
test('requires gas and full funding before the first broadcast', () => {
    assert.throws(() => planPayment([note(100)], 100n, 10n, 0n, false), /Insufficient/);
    assert.throws(() => planPayment([note(100)], 100n, 10n, 0n, true), /Insufficient/);
    assert.throws(() => planPayment([], 0n, 1n, 1n, true), /positive/);
});
test('transparent funding splits at uint64 and excludes spent and foreign notes', () => {
    const max = (1n << 64n) - 1n;
    const parts = planPayment([{ ...note(100), status: 'spent' }, { ...note(100), assetId: '2' }], max + 1n, 10n, max + 21n, true);
    assert.deepEqual(parts.map(p => p.value), [max, 1n]);
    assert.ok(parts.every(p => p.note === null));
});

function fixture() {
    const recipient = { address: '0x1111111111111111111111111111111111111111' };
    const commitment = ethers.toBeHex(shieldedMimcHash([2001n, BigInt(recipient.address), 1n, 50n, 123n]), 32);
    const empty = [ethers.ZeroHash, ethers.ZeroHash, '0x', '0x', '0x', '0x'];
    const envelope = ['0x02', [], [[commitment, ...empty.slice(1)], empty, empty, empty], ethers.ZeroHash, ethers.ZeroHash];
    const transaction = { chainId: 8979, to: '0x00000000000000000000000000000000000000f7', value: 50n, gas: 3000000n, nonce: 0, gasFeeCap: 1n, gasTipCap: 1n, accessList: [], data: ethers.concat([ethers.toUtf8Bytes('TKMSHIELD1'), ethers.encodeRlp(envelope)]) };
    return { response: { transaction, shieldedVersion: 2, outputOpenings: [{ index: 0, recipient: recipient.address, assetId: '1', valueWei: '50', randomness: '123', commitment }] }, expected: { nonce: 0, gasPrice: 1n, part: { value: 50n, sponsor: 0n }, recipient }, envelope };
}
test('accepts a matching V2 deposit and rejects altered fees and recipient', () => {
    const { response, expected } = fixture();
    assert.equal(validateResponse(response, expected), response.transaction);
    assert.throws(() => validateResponse({ ...response, transaction: { ...response.transaction, gasFeeCap: 2n } }, expected), /fees/);
    assert.throws(() => validateResponse(response, { ...expected, recipient: { address: '0x2222222222222222222222222222222222222222' } }), /recipient/);
});
test('rejects a hidden withdrawal even with correct output commitments', () => {
    const { response, expected, envelope } = fixture();
    envelope.push(expected.recipient.address, '0x01');
    response.transaction.data = ethers.concat([ethers.toUtf8Bytes('TKMSHIELD1'), ethers.encodeRlp(envelope)]);
    assert.throws(() => validateResponse(response, expected), /withdrawal/);
});
test('validates the decrypted seed and viewing key before any RPC traffic', async () => {
    const seed = new Uint8Array(32).fill(7);
    const keystore = await createPQKeystore('test-password', { seed, scryptN: 1024 });
    const identity = deriveShieldedIdentity(seed, ethers.getAddress('0x' + keystore.address.replace(/^0x/, '')));
    const wrong = deriveShieldedIdentity(new Uint8Array(32).fill(8), identity.address);
    const options = { keystore, password: 'test-password', intent: { from: wrong.paymentCode, recipient_address: identity.paymentCode, amount: '1' }, rpcURL: 'http://127.0.0.1:1', proverURL: 'http://127.0.0.1:8787', onProgress() {} };
    await assert.rejects(sendTKM(options), /source payment code/);
    await assert.rejects(sendTKM({ ...options, password: 'wrong' }), /incorrect PQ keyfile password/);
});

test('unlocks, proves, signs type 0x06, broadcasts and reports without uploading the keyfile', async () => {
    const seed = new Uint8Array(32).fill(9);
    const keystore = await createPQKeystore('local-only-password', { seed, scryptN: 1024 });
    const source = deriveShieldedIdentity(seed, ethers.getAddress('0x' + keystore.address.replace(/^0x/, '')));
    const destination = deriveShieldedIdentity(new Uint8Array(32).fill(4), fixture().expected.recipient.address);
    const traffic = [], reported = [];
    let raw;
    const server = createServer(async (req, res) => {
        let body = '';
        for await (const chunk of req) body += chunk;
        traffic.push(body);
        res.setHeader('content-type', 'application/json');
        if (req.url === '/healthz') return res.end(JSON.stringify({ ok: true, hasProvingKeyV2: true }));
        if (req.url === '/build-deposit') {
            const request = JSON.parse(body);
            assert.equal(request.from, source.address);
            assert.equal(request.to, destination.address);
            assert.equal(request.amountWei, '50');
            return res.end(JSON.stringify(fixture().response, (_, v) => typeof v === 'bigint' ? ethers.toQuantity(v) : v));
        }
        const requests = JSON.parse(body);
        const respond = request => {
            const values = { eth_chainId: '0x2313', tkmprivacy_shieldedV2Active: true,
                tkmprivacy_shieldedGasSponsorActive: true, eth_blockNumber: '0x0',
                tkmprivacy_commitmentActivationTime: '0x1', eth_getBlockByNumber: { timestamp: '0x0' },
                eth_gasPrice: '0x1', eth_getBalance: '0x989680', eth_getTransactionCount: '0x0' };
            let result = values[request.method];
            if (request.method === 'eth_sendRawTransaction') {
                raw = request.params[0];
                result = ethers.keccak256(raw);
            }
            assert.notEqual(result, undefined, request.method);
            return { jsonrpc: '2.0', id: request.id, result };
        };
        res.end(JSON.stringify(Array.isArray(requests) ? requests.map(respond) : respond(requests)));
    });
    server.listen(0, '127.0.0.1');
    await once(server, 'listening');
    const url = `http://127.0.0.1:${server.address().port}`;
    try {
        const prepared = await sendTKM({ keystore, password: 'local-only-password',
            intent: { from: source.paymentCode, recipient_address: destination.paymentCode, amount: ethers.formatEther(50n) },
            rpcURL: url, proverURL: url, onProgress() {}, prepareOnly: true,
            beforePart() { assert.fail('prepare-only must not create a browser intent'); },
            onSubmitted() { assert.fail('prepare-only must not submit'); } });
        assert.equal(raw, undefined, 'prepare-only broadcast a transaction');
        assert.equal(prepared.amountWei, '50');
        const recipientPaid = await sendTKM({ keystore, password: 'local-only-password',
            intent: { from: source.paymentCode, recipient_address: destination.paymentCode, amount: ethers.formatEther(3000050n) },
            rpcURL: url, proverURL: url, onProgress() {}, prepareOnly: true, recipientPaysGas: true });
        assert.equal(recipientPaid.amountWei, '50');
        assert.equal(recipientPaid.feeWei, '3000000');
        assert.equal(raw, undefined, 'recipient-paid preparation broadcast');

        assert.ok(prepared.raw.startsWith('0x06'));
        assert.equal(prepared.hash, ethers.keccak256(prepared.raw));
        const hashes = await sendTKM({ keystore, password: 'local-only-password',
            intent: { from: source.paymentCode, recipient_address: destination.paymentCode, amount: ethers.formatEther(50n) },
            rpcURL: url, proverURL: url, onProgress() {},
            beforePart: async (amount, count) => { assert.equal(amount, ethers.formatEther(50n)); assert.equal(count, 1); return { id: 'test-intent' }; },
            onSubmitted: async (intent, hash) => reported.push([intent.id, hash]) });
        assert.ok(raw.startsWith('0x06'));
        assert.deepEqual(reported, [['test-intent', hashes[0]]]);
        assert.equal(hashes[0], ethers.keccak256(raw));
        assert.ok(traffic.every(body => !body.includes('local-only-password') && !body.includes(keystore.crypto.ciphertext)));
    } finally {
        server.closeAllConnections();
        await new Promise(resolve => server.close(resolve));
    }
});

test('recipient-paid gas conserves gross budget across shielded and public parts', () => {
    for (const [notes,balance,sponsored] of [[[note(100),note(100)],0n,true],[[],200n,false],[[note(100)],100n,false]]) {
        const parts=planRecipientPaidPayment(notes,150n,10n,balance,sponsored);
        assert.equal(parts.reduce((sum,p)=>sum+p.value+p.fee,0n),150n);
        assert.ok(parts.every(p=>p.value>0n && p.fee===10n));
    }
    const parts=planRecipientPaidPayment([note(100),note(100)],150n,10n,0n,true);
    assert.deepEqual(parts.map(p=>[p.value,p.fee]),[[90n,10n],[40n,10n]]);
    assert.throws(()=>planRecipientPaidPayment([note(100)],10n,10n,0n,true),/Insufficient/);
});
