import { xchacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf';
import { sha512 } from '@noble/hashes/sha512';
import { ml_kem1024 } from '@noble/post-quantum/ml-kem.js';
import { ethers } from 'ethers';

const MAGIC = new TextEncoder().encode('TKPQ');
const CONTEXT_LABEL = new TextEncoder().encode('TKM_PHONE_ANTARTICAL_CONTEXT_V1');
const TRANSCRIPT_LABEL = new TextEncoder().encode('TKM_SHIELD3_ENCRYPTION_TRANSCRIPT_V1');
const KEY_INFO = new TextEncoder().encode('TKM_SHIELD3_XCHACHA20POLY1305_KEY_V1');
const DEVICE_SEED_LABEL = new TextEncoder().encode('TKM_PHONE_ANTARTICAL_DEVICE_SEED_V1');
const VERSION = 3;
const SUITE = 1;
const PURPOSE = 5;
const CHAIN_ID = 8979;
const MAX_PLAINTEXT = 4096;
const HEADER_SIZE = 79;
const KEM_CIPHERTEXT_SIZE = 1568;
const KEM_PUBLIC_KEY_SIZE = 1568;
const XNONCE_SIZE = 24;
const TAG_SIZE = 16;
const ENVELOPE_SIZE = HEADER_SIZE + KEM_CIPHERTEXT_SIZE + XNONCE_SIZE + 2 + MAX_PLAINTEXT + TAG_SIZE;

function asBytes(value, name) {
    try { return value instanceof Uint8Array ? new Uint8Array(value) : ethers.getBytes(value); }
    catch { throw new Error(`${name} must be hexadecimal data`); }
}

function u64(value) {
    const result = new Uint8Array(8);
    new DataView(result.buffer).setBigUint64(0, BigInt(value), false);
    return result;
}

function concat(...parts) {
    const total = parts.reduce((size, part) => size + part.length, 0);
    const result = new Uint8Array(total);
    let offset = 0;
    for (const part of parts) { result.set(part, offset); offset += part.length; }
    return result;
}

function field(value) {
    return concat(u64(value.length), value);
}

function contextCommitment(chainId, from, to, nonce) {
    return sha512(concat(CONTEXT_LABEL, u64(chainId), field(new TextEncoder().encode(from)), field(new TextEncoder().encode(to)), field(nonce)));
}

function header(chainId, from, to, nonce) {
    return concat(MAGIC, new Uint8Array([VERSION, SUITE, PURPOSE]), u64(chainId), contextCommitment(chainId, from, to, nonce));
}

function transcriptKey(sharedSecret, publicKey, envelopeHeader, kemCiphertext) {
    const transcript = sha512(concat(TRANSCRIPT_LABEL, publicKey, envelopeHeader, kemCiphertext));
    return hkdf(sha512, sharedSecret, transcript, KEY_INFO, 32);
}

function validateEnvelope(envelope, chainId, from, to, nonce) {
    if (envelope.length !== ENVELOPE_SIZE) throw new Error('invalid Antartical phone envelope size');
    const expected = header(chainId, from, to, nonce);
    for (let i = 0; i < HEADER_SIZE; i++) if (envelope[i] !== expected[i]) throw new Error('invalid Antartical phone envelope context');
}

// Derive an independent ML-KEM-1024 device seed from the local device signing key.
// The ECDSA key is never sent to the node; only the derived public encapsulation key is registered.
export function derivePhoneEncryptionSeed(devicePrivateKey) {
    const key = asBytes(devicePrivateKey, 'device private key');
    if (key.length !== 32) throw new Error('device private key must be 32 bytes');
    return sha512(concat(DEVICE_SEED_LABEL, key));
}

export function phoneEncryptionPublicKey(seed) {
    const kemSeed = asBytes(seed, 'phone encryption seed');
    if (kemSeed.length !== 64) throw new Error('phone encryption seed must be 64 bytes');
    return ml_kem1024.keygen(kemSeed).publicKey;
}

export function encryptPhoneV2(publicKey, chainId = CHAIN_ID, from, to, nonce, plaintext) {
    const key = asBytes(publicKey, 'recipient ML-KEM public key');
    if (key.length !== KEM_PUBLIC_KEY_SIZE) throw new Error('recipient ML-KEM public key must be 1568 bytes');
    const iv = asBytes(nonce, 'phone nonce');
    if (iv.length === 0) throw new Error('phone nonce is required');
    const message = typeof plaintext === 'string' ? new TextEncoder().encode(plaintext) : asBytes(plaintext, 'plaintext');
    if (message.length > MAX_PLAINTEXT) throw new Error('phone plaintext exceeds 4096 bytes');
    const envelopeHeader = header(chainId, from, to, iv);
    const { cipherText, sharedSecret } = ml_kem1024.encapsulate(key);
    const encryptionKey = transcriptKey(sharedSecret, key, envelopeHeader, cipherText);
    const padded = new Uint8Array(2 + MAX_PLAINTEXT);
    new DataView(padded.buffer).setUint16(0, message.length, false);
    padded.set(message, 2);
    const xnonce = crypto.getRandomValues(new Uint8Array(XNONCE_SIZE));
    const aad = concat(envelopeHeader, cipherText, xnonce);
    const sealed = xchacha20poly1305(encryptionKey, xnonce, aad).encrypt(padded);
    return ethers.hexlify(concat(aad, sealed));
}

export function decryptPhoneV2(seed, envelopeValue, chainId = CHAIN_ID, from, to, nonce) {
    const kemSeed = asBytes(seed, 'phone encryption seed');
    const envelope = asBytes(envelopeValue, 'phone envelope');
    const iv = asBytes(nonce, 'phone nonce');
    if (kemSeed.length !== 64) throw new Error('phone encryption seed must be 64 bytes');
    validateEnvelope(envelope, chainId, from, to, iv);
    const envelopeHeader = envelope.slice(0, HEADER_SIZE);
    const kemCiphertext = envelope.slice(HEADER_SIZE, HEADER_SIZE + KEM_CIPHERTEXT_SIZE);
    const xnonceStart = HEADER_SIZE + KEM_CIPHERTEXT_SIZE;
    const xnonce = envelope.slice(xnonceStart, xnonceStart + XNONCE_SIZE);
    const aad = envelope.slice(0, xnonceStart + XNONCE_SIZE);
    const sealed = envelope.slice(xnonceStart + XNONCE_SIZE);
    const secretKey = ml_kem1024.keygen(kemSeed).secretKey;
    const sharedSecret = ml_kem1024.decapsulate(kemCiphertext, secretKey);
    const publicKey = ml_kem1024.keygen(kemSeed).publicKey;
    const encryptionKey = transcriptKey(sharedSecret, publicKey, envelopeHeader, kemCiphertext);
    let padded;
    try { padded = xchacha20poly1305(encryptionKey, xnonce, aad).decrypt(sealed); }
    catch { throw new Error('invalid Antartical phone envelope authentication'); }
    const length = new DataView(padded.buffer, padded.byteOffset, padded.byteLength).getUint16(0, false);
    if (padded.length !== 2 + MAX_PLAINTEXT || length > MAX_PLAINTEXT) throw new Error('invalid Antartical phone envelope payload');
    for (let i = 2 + length; i < padded.length; i++) if (padded[i] !== 0) throw new Error('invalid Antartical phone envelope padding');
    return padded.slice(2, 2 + length);
}

export const PHONE_V2_ENVELOPE_SIZE = ENVELOPE_SIZE;
export const PHONE_V2_KEM_PUBLIC_KEY_SIZE = KEM_PUBLIC_KEY_SIZE;
