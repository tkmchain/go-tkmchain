import { ethers } from 'ethers';

export const PQ_ALGORITHM = 'ML-DSA-87';
export const PQ_PUBLIC_KEY_BYTES = 2592;
const PQ_ADDRESS_DOMAIN = 'tkmchain:pq-address:v1:';

// Normalize keystore format (handle Geth vs standard)
export function normalizeKeystore(keystore) {
    // If it has Crypto (capital C) instead of crypto, convert it
    if (keystore.Crypto && !keystore.crypto) {
        keystore.crypto = keystore.Crypto;
        delete keystore.Crypto;
    }

    // Ensure address is lowercase without 0x
    if (keystore.address) {
        keystore.address = keystore.address.toLowerCase();
        if (keystore.address.startsWith('0x')) {
            keystore.address = keystore.address.slice(2);
        }
    }

    return keystore;
}

export function isPQKeystore(keystore) {
    try {
        const obj = normalizeKeystore({ ...(typeof keystore === 'string' ? JSON.parse(keystore) : keystore) });
        if (Number(obj?.version) !== 4 || obj?.algorithm !== PQ_ALGORITHM || !isValidKeystore(obj)) {
            return false;
        }

        const declaredAddress = getDeclaredAddress(obj);
        const derivedAddress = getPQAddressFromPublicKey(obj.publicKey);
        return !!declaredAddress && declaredAddress === derivedAddress;
    } catch {
        return false;
    }
}

export function isLegacyKeystore(keystore) {
    try {
        const obj = normalizeKeystore({ ...(typeof keystore === 'string' ? JSON.parse(keystore) : keystore) });
        return Number(obj?.version) === 3 && !obj?.algorithm && !obj?.publicKey && isValidKeystore(obj);
    } catch {
        return false;
    }
}

function getDeclaredAddress(keystore) {
    if (!keystore?.address) return '';
    const address = keystore.address.startsWith('0x') ? keystore.address : '0x' + keystore.address;
    return ethers.isAddress(address) ? ethers.getAddress(address) : '';
}

export function getPQAddressFromPublicKey(publicKey) {
    if (typeof publicKey !== 'string') return '';
    const cleanPublicKey = publicKey.startsWith('0x') ? publicKey.slice(2) : publicKey;
    if (!/^[0-9a-fA-F]+$/.test(cleanPublicKey) || cleanPublicKey.length !== PQ_PUBLIC_KEY_BYTES * 2) {
        return '';
    }

    const digest = ethers.keccak256(ethers.concat([
        ethers.toUtf8Bytes(PQ_ADDRESS_DOMAIN),
        ethers.toUtf8Bytes(PQ_ALGORITHM),
        ethers.getBytes('0x' + cleanPublicKey)
    ]));
    return ethers.getAddress(ethers.dataSlice(digest, 12));
}

export function getKeystoreAddress(keystore) {
    const obj = normalizeKeystore({ ...(typeof keystore === 'string' ? JSON.parse(keystore) : keystore) });
    return isPQKeystore(obj) ? getPQAddressFromPublicKey(obj.publicKey) : getDeclaredAddress(obj);
}

// Validate keystore JSON format
export function isValidKeystore(json) {
    try {
        let obj;
        if (typeof json === 'string') {
            obj = JSON.parse(json);
        } else {
            obj = json;
        }

        const hasVersion = obj.version !== undefined;
        const hasCrypto = obj.crypto !== undefined || obj.Crypto !== undefined;
        const hasCiphertext = obj.crypto?.ciphertext || obj.Crypto?.ciphertext;
        const hasCipher = obj.crypto?.cipher || obj.Crypto?.cipher;
        const hasKdf = obj.crypto?.kdf || obj.Crypto?.kdf;

        return !!(hasVersion && hasCrypto && hasCiphertext && hasCipher && hasKdf);
    } catch {
        return false;
    }
}

// Export wallet as JSON keystore file
export function downloadKeystore(keystoreJson, filename = 'keystore.json') {
    let cleanKeystore;
    if (typeof keystoreJson === 'string') {
        cleanKeystore = JSON.parse(keystoreJson);
    } else {
        cleanKeystore = keystoreJson;
    }

    const blob = new Blob([JSON.stringify(cleanKeystore, null, 2)], {
        type: 'application/json'
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Parse keystore file with better error handling and debug info
export function parseKeystoreFile(file) {
    return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = (e) => {
            try {
                const content = e.target.result;
                let json;

                try {
                    json = JSON.parse(content);
                } catch (parseError) {
                    resolve({
                        success: false,
                        error: 'Invalid JSON format. Please make sure the file is a valid keystore JSON file.'
                    });
                    return;
                }

                // Check for different wallet formats
                if (json.crypto || json.Crypto) {
                    // Standard keystore
                    if (isValidKeystore(json)) {
                        resolve({ success: true, keystore: json });
                    } else {
                        resolve({
                            success: false,
                            error: 'Keystore validation failed. Missing required fields (version, ciphertext, cipher, kdf).'
                        });
                    }
                } else if (json.privateKey || json.private_key) {
                    // Raw private key
                    resolve({
                        success: false,
                        error: 'Raw private keys are not accepted. This wallet supports encrypted ML-DSA-87 keyfiles only.'
                    });
                } else if (json.address && json.encrypted) {
                    // Some custom encrypted format
                    resolve({
                        success: false,
                        error: 'This appears to be an encrypted file but not a standard keystore. Please use a standard keystore JSON file.'
                    });
                } else if (json.addresses || json.wallets) {
                    // Multiple wallets file
                    resolve({
                        success: false,
                        error: 'This appears to be a wallet collection file. Please extract a single keystore.'
                    });
                } else {
                    resolve({
                        success: false,
                        error: 'Unknown file format. Expected a keystore JSON with "crypto" field.'
                    });
                }
            } catch (error) {
                resolve({
                    success: false,
                    error: 'Failed to read file: ' + error.message
                });
            }
        };
        reader.onerror = () => {
            resolve({
                success: false,
                error: 'Failed to read file'
            });
        };
        reader.readAsText(file);
    });
}
