import { ethers } from 'ethers';
import { TkmChainAPI } from './vendor/api.js';
import { normalizeKeystore } from './vendor/keystore.js';
import { decryptPQKeystore, signPQTkmTransactionWithSeed } from './vendor/pq.js';
import { ShieldedProverClient, decodeShieldedPaymentCode, deriveShieldedIdentity, clearShieldedIdentity, scanShieldedNotes, validateProverTransaction, validateV2ProverOutputs, validateV2GasSponsor } from './vendor/shielded.js';

const GAS = 3000000n;
const MAX_NOTE = (1n << 64n) - 1n;
import {acquireWalletOperation} from './operation-lock.js';

export function planPayment(notes, amount, gasCost, balance, sponsorship) {
    let remaining = BigInt(amount), publicBalance = BigInt(balance);
    if (remaining <= 0n) throw new Error('Amount must be positive.');
    const parts = [];
    for (const note of notes.filter(n => n.status === 'available' && Number(n.version) === 2 && BigInt(n.assetId) === 1n)) {
        if (!remaining) break;
        const sponsor = sponsorship && publicBalance < gasCost ? gasCost - publicBalance : 0n;
        if (!sponsorship && publicBalance < gasCost) break;
        const capacity = BigInt(note.noteValueWei) - sponsor;
        if (capacity <= 0n) continue;
        const value = remaining < capacity ? remaining : capacity;
        parts.push({ note, value, sponsor });
        publicBalance -= gasCost - sponsor;
        remaining -= value;
    }
    while (remaining > 0n) {
        const value = remaining < MAX_NOTE ? remaining : MAX_NOTE;
        if (publicBalance < value + gasCost) throw new Error('Insufficient spendable TKM including network gas.');
        parts.push({ note: null, value, sponsor: 0n });
        publicBalance -= value + gasCost;
        remaining -= value;
    }
    return parts;
}

// The requested amount is a total budget: every part pays its own gas.
export function planRecipientPaidPayment(notes, amount, gasCost, balance, sponsorship) {
    let remaining=BigInt(amount), available=BigInt(balance);
    if (remaining<=0n) throw new Error('Amount must be positive.');
    const parts=[];
    for (const note of notes.filter(n=>n.status==='available' && Number(n.version)===2 && BigInt(n.assetId)===1n)) {
        if (!remaining) break;
        if (!sponsorship && available<gasCost) break;
        const sponsor=available<gasCost ? gasCost-available : 0n;
        const capacity=BigInt(note.noteValueWei)+gasCost-sponsor;
        const budget=remaining<capacity ? remaining : capacity;
        if (budget<=gasCost) continue;
        parts.push({note,value:budget-gasCost,sponsor,fee:gasCost,budget});
        available-=gasCost-sponsor;
        remaining-=budget;
    }
    while (remaining>0n) {
        const budget=remaining<MAX_NOTE+gasCost ? remaining : MAX_NOTE+gasCost;
        if (budget<=gasCost || available<budget) throw new Error('Insufficient payout amount or spendable funds after network gas.');
        parts.push({note:null,value:budget-gasCost,sponsor:0n,fee:gasCost,budget});
        available-=budget;remaining-=budget;
    }
    return parts;
}

export function validateResponse(response, { nonce, gasPrice, part, address, recipient }) {
    const tx = response.transaction;
    validateProverTransaction(tx, { chainId: 8979, value: part.note ? 0n : part.value });
    if (BigInt(tx.nonce) !== BigInt(nonce) || BigInt(tx.gas) !== GAS || BigInt(tx.gasFeeCap) !== gasPrice || BigInt(tx.gasTipCap) !== gasPrice || !Array.isArray(tx.accessList) || tx.accessList.length) throw new Error('Proof builder changed transaction fees or nonce.');
    if (validateV2GasSponsor(tx, response) !== part.sponsor) throw new Error('Proof builder changed gas sponsorship.');
    const outputs = [{ index: 0, recipient: recipient.address, assetId: 1n, valueWei: part.value }];
    if (part.note) {
        if (String(response.spentNullifier).toLowerCase() !== part.note.nullifier.toLowerCase()) throw new Error('Proof builder changed the input note.');
        const change = BigInt(part.note.noteValueWei) - part.value - part.sponsor;
        if (change > 0n) outputs.push({ index: 1, recipient: address, assetId: 1n, valueWei: change });
    }
    validateV2ProverOutputs(tx, response, outputs);
    const envelope = ethers.decodeRlp(ethers.getBytes(tx.data).slice(10));
    if ((envelope[6] && envelope[6] !== '0x' && BigInt(envelope[6]) !== 0n) || envelope[1].length !== (part.note ? 1 : 0)) throw new Error('Proof builder changed the spend or withdrawal.');
    if (part.note && ethers.hexlify(envelope[1][0][0]).toLowerCase() !== part.note.nullifier.toLowerCase()) throw new Error('Proof envelope contains the wrong nullifier.');
    return tx;
}

export async function sendTKM({ keystore, password, intent, rpcURL, proverURL, proverToken, onProgress, beforePart, onSubmitted, prepareOnly = false, proverFetch, recipientPaysGas = false, rpcToken = "" }) {
    const release = acquireWalletOperation();
    let seed, identity, provider;
    const hashes = [];
    try {
        const rpc = new URL(rpcURL);
        const local = new URL(proverURL);
        if (!['https:', 'http:'].includes(rpc.protocol)) throw new Error('Enter an HTTP(S) TKM RPC URL.');
        if (local.protocol !== 'https:' && !(local.protocol === 'http:' && (['127.0.0.1', 'localhost', '[::1]'].includes(local.hostname) || (typeof window !== 'undefined' && local.origin === window.location.origin)))) throw new Error('The configured proof builder must use HTTPS.');
        const source = decodeShieldedPaymentCode(intent.from, 8979);
        const recipient = decodeShieldedPaymentCode(intent.deposit_address || intent.recipient_address, 8979);
        if (source.version !== 2 || recipient.version !== 2) throw new Error('TKM requires shield2 payment codes.');
        onProgress('Unlocking keyfile locally...');
        const normalized = normalizeKeystore(keystore);
        seed = await decryptPQKeystore(normalized, password);
        const address = ethers.getAddress('0x' + String(normalized.address).replace(/^0x/i, ''));
        identity = deriveShieldedIdentity(seed, address, 8979);
        if (address !== source.address || ethers.hexlify(identity.viewPublicKey) !== ethers.hexlify(source.viewPublicKey)) throw new Error('Keyfile does not match the source payment code.');
        const connection = new ethers.FetchRequest(rpcURL);
        if (rpcToken) connection.setHeader("X-GUI-Token", rpcToken);
        provider = new ethers.JsonRpcProvider(connection);
        if (BigInt(await provider.send('eth_chainId', [])) !== 8979n) throw new Error('RPC is not TKM chain 8979.');
        const api = new TkmChainAPI(provider);
        if (!await api.shieldedV2Active()) throw new Error('Shielded V2 is not active.');
        const prover = new ShieldedProverClient(proverURL, proverToken, proverFetch);
        const health = await prover.health();
        if (!health.ok || !health.hasProvingKeyV2) throw new Error(health.startupError || 'Proof builder is not ready.');
        const state = { lastScannedBlock: -1, notes: [] };
        await scanShieldedNotes(api, identity, state, (block, latest) => onProgress(`Scanning notes: ${block} / ${latest}`));
        const gasPrice = BigInt(await provider.send('eth_gasPrice', []));
        const balance = BigInt(await provider.send('eth_getBalance', [address, 'latest']));
        const parts = (recipientPaysGas ? planRecipientPaidPayment : planPayment)(state.notes, ethers.parseUnits(intent.amount, 18), GAS * gasPrice, balance, await api.shieldedGasSponsorActive());
        for (let index = 0; index < parts.length; index++) {
            const part = parts[index];
            const nonce = await api.getTransactionCount(address, 'latest');
            if (BigInt(await api.getTransactionCount(address, 'pending')) !== BigInt(nonce)) throw new Error('Source wallet has a pending transaction. Wait for confirmation before retrying.');
            // Refunds change the available gas balance after each confirmation.
            const availableGas = BigInt(await provider.send('eth_getBalance', [address, 'latest']));
            if (part.note) part.sponsor = availableGas < GAS * gasPrice ? GAS * gasPrice - availableGas : 0n;
            onProgress(`Building proof ${index + 1} of ${parts.length}...`);
            const request = { requestId: `exchange-${ethers.hexlify(ethers.randomBytes(16)).slice(2)}`, from: address, to: recipient.address, amountWei: part.value.toString(), recipientViewKey: ethers.hexlify(recipient.viewPublicKey), nonce, gasPriceWei: gasPrice.toString() };
            let response;
            if (part.note) {
                if ((await api.privacyNullifierStatus(part.note.nullifier))?.spent) throw new Error('A selected note was spent. Refresh before retrying.');
                const path = await api.privacyCommitmentPath(part.note.commitment);
                if (!path.found) throw new Error('Selected note is no longer confirmed.');
                const note = { ...part.note, merkleRoot: path.root, merklePath: path.merklePath.map(String), merklePathIndex: path.merklePathIndex.map(v => BigInt(v).toString()) };
                response = await prover.buildTransfer({ ...request, note, changeViewKey: ethers.hexlify(identity.viewPublicKey) });
            } else {
                response = await prover.buildDeposit({ ...request, assetId: '1', ownerSecret: BigInt(address).toString() });
            }
            const tx = validateResponse(response, { nonce, gasPrice, part, address, recipient });
            const partIntent = prepareOnly ? null : await beforePart(ethers.formatUnits(part.value, 18), parts.length);
            onProgress(`Signing and broadcasting part ${index + 1} locally...`);
            const signed = signPQTkmTransactionWithSeed(tx, seed);
            const hash = ethers.keccak256(signed.rawTransaction);
            if (prepareOnly) return { hash, raw: signed.rawTransaction, amountWei: part.value.toString(), feeWei: recipientPaysGas ? part.fee.toString() : '0', gasPriceWei: gasPrice.toString() };
            // Preserve the deterministic hash if the RPC response is lost.
            hashes.push(hash);
            await onSubmitted(partIntent, hash);
            await api.sendRawTransaction(signed.rawTransaction);
            if (index + 1 < parts.length) {
                onProgress(`Waiting for confirmation: ${hash}`);
                let confirmed = false;
                for (let attempt = 0; attempt < 120; attempt++) {
                    const receipt = await api.getTransactionReceipt(hash);
                    if (receipt) {
                        if (BigInt(receipt.status) !== 1n) throw new Error(`Transaction reverted: ${hash}`);
                        confirmed = true;
                        break;
                    }
                    await new Promise(resolve => setTimeout(resolve, 2000));
                }
                if (!confirmed) throw new Error('Confirmation timed out. Check submitted hashes before retrying.');
            }
        }
        return hashes;
    } catch (error) {
        if (hashes.length) error.message += ` Submitted or attempted hashes: ${hashes.join(', ')}. Check these before retrying.`;
        throw error;
    } finally {
        seed?.fill(0);
        clearShieldedIdentity(identity);
        provider?.destroy();
        release();
    }
}
