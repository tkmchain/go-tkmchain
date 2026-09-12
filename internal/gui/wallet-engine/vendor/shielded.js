import { ethers } from 'ethers';
import { x25519 } from '@noble/curves/ed25519';
import { xchacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf';
import { sha256 } from '@noble/hashes/sha256';

export const SHIELDED_POOL_ADDRESS = ethers.getAddress('0x00000000000000000000000000000000000000f7');
export const SHIELDED_NOTE_FORMAT = 'TKM_SHIELDED_NOTE_PAYLOAD_V3';
export const SHIELDED_NOTE_FORMAT_V2 = 'TKM_SHIELDED_NOTE_PAYLOAD_V4';
export const SHIELDED_V2_NOTE_DOMAIN = 2001n;
const BN254_SCALAR_MODULUS = 21888242871839275222246405745257275088548364400416034343698204186575808495617n;
const SHIELDED_ENVELOPE_MAGIC = ethers.toUtf8Bytes('TKMSHIELD1');
const NOTE_KDF_INFO = 'TKM_SHIELDED_NOTE_X25519_XCHACHA20POLY1305_V1';
const VIEW_SALT = 'TKM_SHIELDED_VIEW_SALT_V1';
const VIEW_INFO = 'TKM_SHIELDED_VIEW_X25519_V1';
const STORE_SALT = 'TKM_SHIELDED_STORE_SALT_V1';
const STORE_INFO = 'TKM_SHIELDED_STORE_AES256_V1';

function bytes(value) {
    return ethers.getBytes(value);
}

// Ethers wraps JSON-RPC failures differently across providers and versions.
// Inspect the standard nested error fields so callers can recover from a note
// which another transaction or browser tab has already consumed.
export function isShieldedNullifierSpentError(error) {
    const queue = [error];
    const seen = new Set();
    while (queue.length > 0) {
        const current = queue.shift();
        if (current == null || seen.has(current)) continue;
        if (typeof current === 'string') {
            if (/nullifier already spent/i.test(current)) return true;
            continue;
        }
        if (typeof current !== 'object') continue;
        seen.add(current);
        for (const key of ['message', 'shortMessage', 'reason', 'error', 'info', 'cause']) {
            if (current[key] != null) queue.push(current[key]);
        }
    }
    return false;
}

let mimcConstants = null;

function bn254(value) {
    const reduced = BigInt(value) % BN254_SCALAR_MODULUS;
    return reduced < 0n ? reduced + BN254_SCALAR_MODULUS : reduced;
}

function pow5(value) {
    const square = value * value % BN254_SCALAR_MODULUS;
    return square * square % BN254_SCALAR_MODULUS * value % BN254_SCALAR_MODULUS;
}

function getMimcConstants() {
    if (mimcConstants) return mimcConstants;
    const constants = [];
    let round = ethers.keccak256(ethers.toUtf8Bytes('seed'));
    for (let i = 0; i < 110; i++) {
        round = ethers.keccak256(round);
        constants.push(bn254(round));
    }
    mimcConstants = constants;
    return constants;
}

// This exactly matches gnark-crypto's BN254 MiMC Miyaguchi-Preneel hash.
export function shieldedMimcHash(fields) {
    let state = 0n;
    for (const field of fields) {
        const input = bn254(field);
        let message = input;
        for (const constant of getMimcConstants()) {
            message = pow5(bn254(message + state + constant));
        }
        message = bn254(message + state);
        state = bn254(message + state + input);
    }
    return state;
}

function normalizedFieldHex(value) {
    return ethers.toBeHex(bn254(value), 32).toLowerCase();
}

// A V2 proof server is untrusted transaction-construction infrastructure. The
// browser recomputes recipient-bound commitments from the returned openings and
// compares them with the exact envelope it is about to sign.
export function validateV2ProverOutputs(transaction, response, expectedOutputs) {
    if (Number(response?.shieldedVersion) !== 2) {
        throw new Error('proof builder did not return a Shielded V2 response');
    }
    const data = bytes(transaction?.data || '0x');
    if (data.length <= SHIELDED_ENVELOPE_MAGIC.length || !SHIELDED_ENVELOPE_MAGIC.every((value, index) => data[index] === value)) {
        throw new Error('proof builder returned an invalid shielded envelope');
    }
    const envelope = ethers.decodeRlp(data.slice(SHIELDED_ENVELOPE_MAGIC.length));
    if (!Array.isArray(envelope) || BigInt(envelope[0]) !== 2n || !Array.isArray(envelope[2]) || envelope[2].length !== 4) {
        throw new Error('proof builder returned an invalid Shielded V2 envelope');
    }
    const openings = Array.isArray(response.outputOpenings) ? response.outputOpenings : [];
    if (openings.length !== expectedOutputs.length) {
        throw new Error('proof builder returned incomplete V2 output openings');
    }
    const seen = new Set();
    for (const expected of expectedOutputs) {
        const opening = openings.find((item) => Number(item.index) === Number(expected.index));
        if (!opening || seen.has(Number(opening.index))) throw new Error('proof builder returned an invalid V2 output index');
        seen.add(Number(opening.index));
        const index = Number(opening.index);
        if (!Number.isInteger(index) || index < 0 || index >= 4) throw new Error('proof builder returned an invalid V2 output index');
        const recipient = ethers.getAddress(opening.recipient);
        if (recipient !== ethers.getAddress(expected.recipient) || BigInt(opening.valueWei) !== BigInt(expected.valueWei) || BigInt(opening.assetId) !== BigInt(expected.assetId)) {
            throw new Error('proof builder changed the requested V2 recipient, value, or asset');
        }
        const commitment = normalizedFieldHex(shieldedMimcHash([
            SHIELDED_V2_NOTE_DOMAIN,
            BigInt(recipient),
            BigInt(opening.assetId),
            BigInt(opening.valueWei),
            BigInt(opening.randomness)
        ]));
        const envelopeCommitment = ethers.hexlify(envelope[2][index][0]).toLowerCase();
        if (commitment !== String(opening.commitment).toLowerCase() || commitment !== envelopeCommitment) {
            throw new Error('proof builder returned a V2 output commitment that does not match its opening');
        }
    }
    return transaction;
}

export function validateV2Withdrawal(transaction, response, { recipient, valueWei, expectedOutputs = [] }) {
	validateV2ProverOutputs(transaction, response, expectedOutputs);
	const data = bytes(transaction?.data || '0x');
	const envelope = ethers.decodeRlp(data.slice(SHIELDED_ENVELOPE_MAGIC.length));
	if (!Array.isArray(envelope) || envelope.length < 7) {
		throw new Error('proof builder omitted the shielded withdrawal fields');
	}
	let returnedRecipient;
	try {
		returnedRecipient = ethers.getAddress(ethers.hexlify(envelope[5]));
	} catch {
		throw new Error('proof builder returned an invalid withdrawal recipient');
	}
	if (returnedRecipient !== ethers.getAddress(recipient) || BigInt(envelope[6]) !== BigInt(valueWei)) {
		throw new Error('proof builder changed the withdrawal recipient or value');
	}
	return transaction;
}

export function validateV2GasSponsor(transaction, response) {
	const data = bytes(transaction?.data || '0x');
	if (data.length <= SHIELDED_ENVELOPE_MAGIC.length || !SHIELDED_ENVELOPE_MAGIC.every((value, index) => data[index] === value)) {
		throw new Error('proof builder returned an invalid shielded envelope');
	}
	const envelope = ethers.decodeRlp(data.slice(SHIELDED_ENVELOPE_MAGIC.length));
	const envelopeSponsor = Array.isArray(envelope) && envelope.length >= 8 && envelope[7] !== '0x'
		? BigInt(envelope[7])
		: 0n;
	const responseSponsor = BigInt(response?.gasSponsorWei || 0);
	const maxGasCost = BigInt(transaction.gas) * BigInt(transaction.gasFeeCap);
	if (envelopeSponsor !== responseSponsor || envelopeSponsor < 0n || envelopeSponsor > maxGasCost) {
		throw new Error('proof builder returned an invalid shielded gas sponsorship');
	}
	return envelopeSponsor;
}

// The proof builder is untrusted. Ensure the exact application action returned
// by tkmdomain/emailvm is bound into the spend before the PQ transaction is
// signed in the browser.
export function validateShieldedApplicationData(transaction, applicationData) {
    const expected = ethers.hexlify(applicationData).toLowerCase();
    const data = bytes(transaction?.data || '0x');
    if (data.length <= SHIELDED_ENVELOPE_MAGIC.length || !SHIELDED_ENVELOPE_MAGIC.every((value, index) => data[index] === value)) {
        throw new Error('proof builder returned an invalid shielded envelope');
    }
    const envelope = ethers.decodeRlp(data.slice(SHIELDED_ENVELOPE_MAGIC.length));
    const spends = Array.isArray(envelope) ? envelope[1] : null;
    const spend = Array.isArray(spends) && spends.length > 0 ? spends[0] : null;
    if (!Array.isArray(spend) || spend.length < 4 || ethers.hexlify(spend[3]).toLowerCase() !== expected) {
        throw new Error('proof builder changed or omitted the domain/mail application data');
    }
    return transaction;
}

function base64urlEncode(data) {
    const binary = Array.from(data, (value) => String.fromCharCode(value)).join('');
    return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

function base64urlDecode(value) {
    const normalized = value.replaceAll('-', '+').replaceAll('_', '/');
    const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4);
    return Uint8Array.from(atob(padded), (char) => char.charCodeAt(0));
}

export function deriveShieldedIdentity(seed, address, chainId = 8979) {
    const normalizedAddress = ethers.getAddress(address);
    const viewPrivateKey = hkdf(
        sha256,
        seed,
        ethers.toUtf8Bytes(VIEW_SALT),
        ethers.toUtf8Bytes(VIEW_INFO),
        32
    );
    const viewPublicKey = x25519.getPublicKey(viewPrivateKey);
    const storageKey = hkdf(
        sha256,
        seed,
        ethers.toUtf8Bytes(STORE_SALT),
        ethers.toUtf8Bytes(STORE_INFO),
        32
    );
    const paymentCodeV1 = encodeShieldedPaymentCode({
        chainId,
        address: normalizedAddress,
        viewPublicKey,
        version: 1
    });
    const paymentCodeV2 = encodeShieldedPaymentCode({
        chainId,
        address: normalizedAddress,
        viewPublicKey,
        version: 2
    });
    return { address: normalizedAddress, chainId, viewPrivateKey, viewPublicKey, storageKey, paymentCode: paymentCodeV2, paymentCodeV1, paymentCodeV2 };
}

export function clearShieldedIdentity(identity) {
    if (!identity) return;
    identity.viewPrivateKey?.fill(0);
    identity.storageKey?.fill(0);
}

export function encodeShieldedPaymentCode({ chainId, address, viewPublicKey, version = 2 }) {
    if (version !== 1 && version !== 2) throw new Error('unsupported shielded payment-code version');
    const payload = {
        v: version,
        c: Number(chainId),
        a: ethers.getAddress(address),
        k: ethers.hexlify(viewPublicKey).slice(2)
    };
    return `tkmshield${version}.` + base64urlEncode(ethers.toUtf8Bytes(JSON.stringify(payload)));
}

export function decodeShieldedPaymentCode(code, expectedChainId = 8979) {
    const match = typeof code === 'string' ? code.match(/^tkmshield([12])\./) : null;
    if (!match) {
        throw new Error('recipient must be a tkmshield1 or tkmshield2 payment code');
    }
    const version = Number(match[1]);
    let payload;
    try {
        payload = JSON.parse(ethers.toUtf8String(base64urlDecode(code.slice(`tkmshield${version}.`.length))));
    } catch {
        throw new Error('invalid shielded payment code');
    }
    if (payload.v !== version || Number(payload.c) !== Number(expectedChainId)) {
        throw new Error(`shielded payment code is not for chain ${expectedChainId}`);
    }
    const address = ethers.getAddress(payload.a);
    const viewPublicKey = bytes('0x' + String(payload.k || '').replace(/^0x/, ''));
    if (viewPublicKey.length !== 32) throw new Error('shielded payment code has an invalid viewing public key');
    return { version, chainId: Number(payload.c), address, viewPublicKey };
}

export function assertSelfShieldedRecipient(recipient, identity) {
    const selfAddress = identity?.address ? ethers.getAddress(identity.address) : '';
    const recipientAddress = ethers.getAddress(recipient.address);
    const selfViewKey = identity?.viewPublicKey ? ethers.hexlify(identity.viewPublicKey).toLowerCase() : '';
    const recipientViewKey = ethers.hexlify(recipient.viewPublicKey).toLowerCase();
    if (recipientAddress !== selfAddress || recipientViewKey !== selfViewKey) {
        throw new Error('third-party shielded payments require the recipient-bound V2 circuit; V1 is limited to self-shielding');
    }
    return recipient;
}

function noteKeyMaterial(viewPrivateKey, ephemeralPublicKey, commitment) {
    const shared = x25519.getSharedSecret(viewPrivateKey, ephemeralPublicKey);
    return hkdf(
        sha256,
        shared,
        bytes(commitment),
        ethers.toUtf8Bytes(NOTE_KDF_INFO),
        33
    );
}

export function decryptShieldedOutput(output, identity) {
    const ephemeralPublicKey = bytes(output.ephemeralPubKey);
    const ciphertext = bytes(output.encryptedPayload);
    const nonce = bytes(output.nonce);
    const commitment = ethers.hexlify(output.commitment);
    if (ephemeralPublicKey.length !== 32 || nonce.length !== 24 || bytes(output.viewTag).length !== 1) return null;
    let keyMaterial;
    try {
        keyMaterial = noteKeyMaterial(identity.viewPrivateKey, ephemeralPublicKey, commitment);
        if (keyMaterial[32] !== bytes(output.viewTag)[0]) return null;
        if (ethers.keccak256(ciphertext).toLowerCase() !== String(output.payloadHash).toLowerCase()) return null;
        const aad = bytes(ethers.concat([ethers.toUtf8Bytes(NOTE_KDF_INFO), bytes(commitment)]));
        const plaintext = xchacha20poly1305(keyMaterial.slice(0, 32), nonce, aad).decrypt(ciphertext);
        const opening = JSON.parse(ethers.toUtf8String(plaintext));
        if (opening.format !== SHIELDED_NOTE_FORMAT && opening.format !== SHIELDED_NOTE_FORMAT_V2) return null;
        const version = Number(opening.version || 1);
        if (version !== 1 && version !== 2) return null;
        if (String(opening.commitment).toLowerCase() !== commitment.toLowerCase()) return null;
        if (ethers.getAddress(opening.recipient) !== identity.address) return null;
        if (BigInt(opening.noteValueWei) <= 0n) return null;
        return { ...opening, version };
    } catch {
        return null;
    } finally {
        keyMaterial?.fill(0);
    }
}

export class ShieldedProverClient {
    constructor(url, bearerToken, requestFetch = globalThis.fetch) {
        this.url = String(url || '').replace(/\/$/, '');
        this.bearerToken = String(bearerToken || '').trim();
        this.requestFetch = requestFetch;
    }

    async request(path, body = null) {
        if (!this.url) throw new Error('configure the local proof-builder URL');
        const response = await this.requestFetch(this.url + path, {
            method: body == null ? 'GET' : 'POST',
            headers: {
                ...(this.bearerToken ? { authorization: `Bearer ${this.bearerToken}` } : {}),
                ...(body == null ? {} : { 'content-type': 'application/json' })
            },
            ...(body == null ? {} : { body: JSON.stringify(body) })
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok || payload.error) throw new Error(payload.error || `proof builder returned HTTP ${response.status}`);
        return payload;
    }

    health() { return this.request('/healthz'); }
    buildDeposit(request) { return this.request('/build-deposit', request); }
    buildTransfer(request) { return this.request('/build-transfer', request); }
	buildWithdrawal(request) { return this.request('/build-withdrawal', request); }
}

export function validateProverTransaction(transaction, { chainId = 8979, value = null } = {}) {
    if (!transaction || Number(BigInt(transaction.chainId)) !== Number(chainId)) {
        throw new Error('proof builder returned the wrong chain ID');
    }
    if (ethers.getAddress(transaction.to) !== SHIELDED_POOL_ADDRESS) {
        throw new Error('proof builder returned the wrong shielded pool address');
    }
    if (!String(transaction.data || '').startsWith(ethers.hexlify(ethers.toUtf8Bytes('TKMSHIELD1')))) {
        throw new Error('proof builder did not return a TKMSHIELD1 envelope');
    }
    if (value != null && BigInt(transaction.value) !== BigInt(value)) {
        throw new Error('proof builder returned an unexpected public value');
    }
    return transaction;
}

function mergeNote(notes, note) {
    const index = notes.findIndex((item) => item.commitment?.toLowerCase() === note.commitment.toLowerCase());
    if (index >= 0) notes[index] = { ...notes[index], ...note };
    else notes.push(note);
}

async function firstBlockAtOrAfter(api, latest, timestamp) {
    if (timestamp <= 0n) return 0;
    const latestBlock = await api.getBlock(latest, false);
    if (!latestBlock || BigInt(latestBlock.timestamp) < timestamp) return latest + 1;
    let low = 0;
    let high = latest;
    while (low < high) {
        const middle = Math.floor((low + high) / 2);
        const block = await api.getBlock(middle, false);
        if (!block) return 0;
        if (BigInt(block.timestamp) >= timestamp) high = middle;
        else low = middle + 1;
    }
    return low;
}

export async function scanShieldedNotes(api, identity, state, onProgress = null) {
    const latest = await api.getBlockNumber();
    const priorLast = Number(state.lastScannedBlock ?? -1);
    let from;
    if (priorLast < 0) {
        let activationTime = 0n;
        try {
            activationTime = BigInt(await api.privacyCommitmentActivationTime());
        } catch {
            // Older nodes do not expose the activation-time helper.
        }
        from = await firstBlockAtOrAfter(api, latest, activationTime);
    } else {
        from = Math.max(0, priorLast - 12);
    }

    // A wallet may have scanned past the block which consumed a stale local
    // note without retaining the transaction's change output (for example,
    // after clearing browser data in another tab). Rewind to that canonical
    // spend so the same scan can recover any output encrypted back to us.
    const existingNotes = state.notes || [];
    for (const note of existingNotes) {
        if (!note.nullifier) continue;
        const nullifier = await api.privacyNullifierStatus(note.nullifier);
        if (!nullifier?.spent) continue;
        note.status = 'spent';
        try {
            const spentHeight = Number(BigInt(nullifier.spentHeight));
            if (Number.isSafeInteger(spentHeight) && spentHeight >= 0) {
                from = Math.min(from, Math.max(0, spentHeight - 1));
            }
        } catch {
            // Older nodes report only the spent flag; status is still useful.
        }
    }

    const notes = existingNotes.filter((note) => !(
        note.source === 'scan' && Number(note.createdBlock ?? -1) >= from
    ));
    state.lastScannedBlock = from - 1;
    while (from <= latest) {
        const to = Math.min(latest, from + 2047);
        const outputs = await api.privacyShieldedOutputs(from, to);
        for (const output of outputs || []) {
            const opening = decryptShieldedOutput(output, identity);
            if (!opening) continue;
            const path = await api.privacyCommitmentPath(opening.commitment);
            const nullifier = await api.privacyNullifierStatus(opening.nullifier);
            mergeNote(notes, {
                id: `scan-${String(output.transactionHash).slice(2, 14)}-${Number(BigInt(output.outputIndex))}`,
                commitment: opening.commitment,
                version: Number(opening.version || 1),
                ownerSecret: opening.ownerSecret || '',
                noteRandomness: opening.noteRandomness,
                noteValueWei: opening.noteValueWei,
                assetId: opening.assetId,
                nullifier: opening.nullifier,
                merklePath: (path.merklePath || []).map(String),
                merklePathIndex: (path.merklePathIndex || []).map((value) => BigInt(value).toString()),
                merkleRoot: path.root || '',
                createdTxHash: output.transactionHash,
                createdBlock: Number(BigInt(output.blockNumber)),
                source: 'scan',
                status: nullifier?.spent ? 'spent' : (path.found ? 'available' : 'pending')
            });
        }
        state.lastScannedBlock = to;
        onProgress?.(to, latest);
        from = to + 1;
    }

    for (const note of notes) {
        if (!note.nullifier) continue;
        const nullifier = await api.privacyNullifierStatus(note.nullifier);
        if (nullifier?.spent) note.status = 'spent';
        if (note.status === 'pendingSpent') continue;
        if (note.status === 'pending') {
            const path = await api.privacyCommitmentPath(note.commitment);
            if (path?.found) {
                note.merklePath = (path.merklePath || []).map(String);
                note.merklePathIndex = (path.merklePathIndex || []).map((value) => BigInt(value).toString());
                note.merkleRoot = path.root;
                note.status = 'available';
            }
        }
    }
    state.notes = notes;
    return state;
}

export function shieldedBalance(notes) {
    return (notes || []).reduce((total, note) => note.status === 'available' ? total + BigInt(note.noteValueWei) : total, 0n);
}
