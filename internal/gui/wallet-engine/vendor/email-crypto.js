import { x25519 } from '@noble/curves/ed25519';
import { xchacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf';
import { sha256 } from '@noble/hashes/sha256';

const encoder = new TextEncoder();
const decoder = new TextDecoder();
const EMAIL_SALT = encoder.encode('TKM_EMAILVM_X25519_SALT_V1');
const EMAIL_INFO = encoder.encode('TKM_EMAILVM_X25519_KEY_V1');
const MESSAGE_INFO = encoder.encode('TKM_EMAILVM_XCHACHA20POLY1305_V1');

function concat(...items) {
    const length = items.reduce((sum, item) => sum + item.length, 0);
    const out = new Uint8Array(length);
    let offset = 0;
    for (const item of items) {
        out.set(item, offset);
        offset += item.length;
    }
    return out;
}

function context(from, to) {
    return encoder.encode(`${String(from).toLowerCase()}\n${String(to).toLowerCase()}`);
}

export function deriveEmailIdentity(seed) {
    const privateKey = hkdf(sha256, seed, EMAIL_SALT, EMAIL_INFO, 32);
    const publicKey = x25519.getPublicKey(privateKey);
    return { privateKey, publicKey };
}

export function emailIdentityFromPrivateKey(value) {
    const privateKey = new Uint8Array(value);
    if (privateKey.length !== 32) {
        privateKey.fill(0);
        throw new Error('EmailVM X25519 private key must be exactly 32 bytes');
    }
    try {
        return { privateKey, publicKey: x25519.getPublicKey(privateKey) };
    } catch (error) {
        privateKey.fill(0);
        throw new Error(`invalid EmailVM X25519 private key: ${error.message}`);
    }
}

function secureRandom(out) {
    if (!globalThis.crypto?.getRandomValues) throw new Error('secure browser randomness is unavailable; use HTTPS');
    return globalThis.crypto.getRandomValues(out);
}

function messageKey(privateKey, peerPublicKey, from, to) {
    const shared = x25519.getSharedSecret(privateKey, peerPublicKey);
    try {
        return hkdf(sha256, shared, context(from, to), MESSAGE_INFO, 32);
    } finally {
        shared.fill(0);
    }
}

export function encryptEmailMessage(privateKey, recipientPublicKey, from, to, plaintext, randomBytes = secureRandom) {
    const nonce = randomBytes(new Uint8Array(24));
    const aad = concat(MESSAGE_INFO, context(from, to));
    const key = messageKey(privateKey, recipientPublicKey, from, to);
    try {
        const ciphertext = xchacha20poly1305(key, nonce, aad).encrypt(encoder.encode(plaintext));
        return { ciphertext, nonce };
    } finally {
        key.fill(0);
    }
}

export function decryptEmailMessage(privateKey, senderPublicKey, from, to, ciphertext, nonce) {
    const aad = concat(MESSAGE_INFO, context(from, to));
    const key = messageKey(privateKey, senderPublicKey, from, to);
    try {
        return decoder.decode(xchacha20poly1305(key, nonce, aad).decrypt(ciphertext));
    } finally {
        key.fill(0);
    }
}

export function clearEmailIdentity(identity) {
    identity?.privateKey?.fill(0);
}
