import { ethers } from 'ethers';

export class TkmChainAPI {
    constructor(provider) {
        this.provider = provider;
        this.rpcUrl = provider?._getConnection?.()?.url || provider?.connection?.url || globalThis.location?.href || "";
    }

    // === Rotating King RPC Methods ===

    async rkAdd(address, lockedAmount = 0) {
        const params = [address];
        if (lockedAmount > 0) {
            params.push(lockedAmount.toString());
        }
        const result = await this.provider.send('rk_add', params);
        return result;
    }

    async rkList() {
        const result = await this.provider.send('rk_list', []);
        return result;
    }

    async rkStatus(address) {
        if (!address) return null;
        const result = await this.provider.send('rk_status', [address]);
        return result;
    }

    async rkGetKingStats() {
        const result = await this.provider.send('rk_getKingStats', []);
        return result;
    }

    // === Get all king info ===

    async getKingInfo() {
        try {
            const kings = await this.rkList();
            console.log('Kings from rk_list:', kings);

            let stats = null;
            try {
                stats = await this.rkGetKingStats();
                console.log('Stats from rk_getKingStats:', stats);
            } catch (e) {
                console.warn('rk_getKingStats not available:', e.message);
            }

            let status = null;
            let currentKing = '—';
            let mainKing = '—';
            let nextKing = '—';
            let nextRotationHeight = '—';
            let blocksUntilRotation = '—';
            let rotationInterval = '—';
            let totalKings = kings ? kings.length : 0;
            let registeredKings = 0;

            if (kings && kings.length > 0) {
                registeredKings = kings.filter(k => k.registered).length;
                const current = kings.find(k => k.current);
                const next = kings.find(k => k.next);
                currentKing = current ? current.address : '—';
                nextKing = next ? next.address : '—';

                for (const king of kings) {
                    if (king.nextRotationHeight) {
                        nextRotationHeight = king.nextRotationHeight;
                        break;
                    }
                }

                try {
                    if (current) {
                        status = await this.rkStatus(current.address);
                        if (status) {
                            mainKing = status.mainKing || '—';
                            rotationInterval = status.rotationInterval || '—';
                            blocksUntilRotation = status.blocksUntilRotation || '—';
                            if (status.totalKings) totalKings = status.totalKings;
                            if (status.registeredKings) registeredKings = status.registeredKings;
                        }
                    }
                } catch (e) {
                    console.warn('rk_status failed:', e.message);
                }
            }

            if (stats) {
                if (stats.totalKings) totalKings = stats.totalKings;
                if (stats.registeredKings) registeredKings = stats.registeredKings;
            }

            return {
                status: status,
                kings: kings || [],
                stats: stats,
                currentKing: currentKing,
                mainKing: mainKing,
                nextKing: nextKing,
                nextRotationHeight: nextRotationHeight,
                blocksUntilRotation: blocksUntilRotation,
                rotationInterval: rotationInterval,
                totalKings: totalKings,
                registeredKings: registeredKings,
                currentBlock: 0
            };
        } catch (error) {
            console.error('Error getting king info:', error);
            return {
                status: null,
                kings: [],
                stats: null,
                currentKing: '—',
                mainKing: '—',
                nextKing: '—',
                nextRotationHeight: '—',
                blocksUntilRotation: '—',
                rotationInterval: '—',
                totalKings: 0,
                registeredKings: 0,
                currentBlock: 0
            };
        }
    }

    // === Standard Ethereum Methods ===

    async getBlockNumber() {
        try {
            const result = await this.provider.send('eth_blockNumber', []);
            return parseInt(result, 16);
        } catch (error) {
            console.error('getBlockNumber failed:', error);
            return 0;
        }
    }

    async getBalance(address) {
        try {
            const result = await this.provider.send('eth_getBalance', [address, 'latest']);
            return result;
        } catch (error) {
            console.warn('getBalance failed:', error.message);
            try {
                const result = await this.provider.send('eth_getBalance', [address, 'pending']);
                return result;
            } catch (e2) {
                console.warn('getBalance with pending also failed:', e2.message);
                return '0x0';
            }
        }
    }

    async getGasPrice() {
        try {
            const result = await this.provider.send('eth_gasPrice', []);
            return result;
        } catch (error) {
            console.warn('getGasPrice failed:', error.message);
            return '0x3b9aca00';
        }
    }

    async getTransactionCount(address, blockTag = 'pending') {
        return this.provider.send('eth_getTransactionCount', [address, blockTag]);
    }

    async sendRawTransaction(rawTransaction) {
        return this.provider.send('eth_sendRawTransaction', [rawTransaction]);
    }

    async transactionBucket() {
        return this.provider.send('tkmprivacy_transactionBucket', []);
    }

    async beginTransactionBatch(expectedCount, firstRawTransaction) {
        return this.provider.send('tkmprivacy_beginTransactionBatch', [
            ethers.toQuantity(expectedCount),
            firstRawTransaction
        ]);
    }

    async queueRawTransaction(batchId, index, rawTransaction) {
        return this.provider.send('tkmprivacy_queueRawTransaction', [
            batchId,
            ethers.toQuantity(index),
            rawTransaction
        ]);
    }

    async transactionBatchStatus(batchId) {
        return this.provider.send('tkmprivacy_transactionBatchStatus', [batchId]);
    }

    async cancelTransactionBatch(batchId, cancelToken) {
        return this.provider.send('tkmprivacy_cancelTransactionBatch', [batchId, cancelToken]);
    }

    async exportPQAccount(address, passphrase, newPassphrase = passphrase) {
        return this.provider.send('tkm_exportPQAccount', [address, passphrase, newPassphrase]);
    }

    async importLegacyKeyfileWithPassphrase(keyfile, passphrase) {
        const keyfileBytes = ethers.hexlify(ethers.toUtf8Bytes(JSON.stringify(keyfile)));
        return this.provider.send('tkm_importLegacyKeyfileWithPassphrase', [keyfileBytes, passphrase]);
    }

    async preparePQMigrationWithPassphrase(address, passphrase) {
        return this.provider.send('tkm_preparePQMigrationWithPassphrase', [address, passphrase]);
    }

    async sendMigrationToPQWithPassphrase(txArgs, publicKey, passphrase) {
        return this.provider.send('tkm_sendMigrationToPQWithPassphrase', [txArgs, publicKey, passphrase]);
    }

    async pqMigrationGas(publicKey) {
        return this.provider.send('tkm_pqMigrationGas', [publicKey]);
    }

    async getMigrationGasPrice() {
        return this.provider.send('eth_gasPrice', []);
    }

    async accountAlgorithm(address) {
        return this.provider.send('tkm_accountAlgorithm', [address]);
    }

    async privacyCommitmentActive() {
        return this.provider.send('tkmprivacy_commitmentActive', []);
    }

    async privacyCommitmentActivationTime() {
        return this.provider.send('tkmprivacy_commitmentActivationTime', []);
    }

    async shieldedV2ActivationTime() {
        return this.provider.send('tkmprivacy_shieldedV2ActivationTime', []);
    }

    async shieldedV2Active() {
        return this.provider.send('tkmprivacy_shieldedV2Active', []);
    }

    async shieldedGasSponsorActivationTime() {
        return this.provider.send('tkmprivacy_shieldedGasSponsorActivationTime', []);
    }

    async shieldedGasSponsorActive() {
        return this.provider.send('tkmprivacy_shieldedGasSponsorActive', []);
    }

    async privacyDefaults() {
        return this.provider.send('tkmprivacy_defaults', []);
    }

    async privacyShieldedOutputs(fromBlock, toBlock) {
        return this.provider.send('tkmprivacy_shieldedOutputs', [
            ethers.toQuantity(fromBlock),
            ethers.toQuantity(toBlock)
        ]);
    }

    async privacyCommitmentPath(commitment) {
        return this.provider.send('tkmprivacy_commitmentPath', [commitment]);
    }

    async privacyNullifierStatus(nullifier) {
        return this.provider.send('tkmprivacy_nullifierStatus', [nullifier]);
    }

    async privacyStatus(address) {
        return this.provider.send('tkmprivacy_status', [address]);
    }

    async getTransactionReceipt(txHash) {
        try {
            const result = await this.provider.send('eth_getTransactionReceipt', [txHash]);
            return result;
        } catch (error) {
            console.warn('getTransactionReceipt failed:', error.message);
            return null;
        }
    }

    async estimateGas(tx) {
        try {
            const result = await this.provider.send('eth_estimateGas', [tx]);
            return result;
        } catch (error) {
            console.warn('estimateGas failed:', error.message);
            return '0x5208';
        }
    }

    async getBlock(blockNumber, includeTx = true) {
        try {
            const hexBlock = typeof blockNumber === 'number' ?
                '0x' + blockNumber.toString(16) :
                blockNumber;
            const result = await this.provider.send('eth_getBlockByNumber', [
                hexBlock,
                includeTx
            ]);
            return result;
        } catch (error) {
            console.warn('getBlock failed:', error.message);
            return null;
        }
    }

    async getTransactionByHash(txHash) {
        try {
            const result = await this.provider.send('eth_getTransactionByHash', [txHash]);
            return result;
        } catch (error) {
            console.warn('getTransactionByHash failed:', error.message);
            return null;
        }
    }

    async getIndexedTransactionHistory(address, limit = 100) {
        const base = new URL(this.rpcUrl || globalThis.location?.href || "");
        const endpoint = `${base.origin}/api/history/${encodeURIComponent(address)}?limit=${encodeURIComponent(limit)}`;
        const response = await fetch(endpoint, {
            method: 'GET',
            headers: { 'Accept': 'application/json' },
            cache: 'no-store'
        });
        if (!response.ok) {
            throw new Error(`history indexer returned HTTP ${response.status}`);
        }
        const body = await response.json();
        return Array.isArray(body.transactions) ? body.transactions : [];
    }

    // === Get transaction history using multiple methods ===
    async getTransactionHistory(address, fromBlock = 0, toBlock = 'latest') {
        try {
            const txs = [];

            // Get the current block number
            let endBlock = toBlock;
            if (toBlock === 'latest') {
                endBlock = await this.getBlockNumber();
            }

            // If we have very few blocks, scan all of them
            const startBlock = Math.max(0, fromBlock);
            const range = endBlock - startBlock;

            // Only scan the last 1000 blocks or the entire chain if it's small
            let scanStart = startBlock;
            if (range > 1000) {
                scanStart = endBlock - 1000;
            }
            console.log(`Scanning blocks from ${scanStart} to ${endBlock}`);

            // Method 1: Try to get transactions from blocks
            for (let i = scanStart; i <= endBlock; i++) {
                try {
                    const block = await this.getBlock(i, true);
                    if (!block || !block.transactions) continue;

                    for (const tx of block.transactions) {
                        const fromMatch = tx.from && tx.from.toLowerCase() === address.toLowerCase();
                        const toMatch = tx.to && tx.to.toLowerCase() === address.toLowerCase();

                        if (fromMatch || toMatch) {
                            let receipt = null;
                            try {
                                receipt = await this.getTransactionReceipt(tx.hash);
                            } catch (e) {
                                // Receipt might not be available yet
                            }

                            const direction = fromMatch ? 'out' : 'in';
                            txs.push({
                                hash: tx.hash,
                                from: tx.from,
                                to: tx.to || 'Contract Creation',
                                value: tx.value ? ethers.formatEther(tx.value) : '0',
                                direction: direction,
                                blockNumber: block.number,
                                timestamp: block.timestamp,
                                status: receipt ? (receipt.status === '0x1' ? 'confirmed' : 'failed') : 'pending',
                                gasUsed: receipt ? receipt.gasUsed : '—',
                                gasPrice: tx.gasPrice ? ethers.formatEther(tx.gasPrice) : '—',
                                nonce: tx.nonce
                            });
                        }
                    }
                } catch (e) {
                    // Skip blocks that can't be accessed
                }
            }

            // Method 2: Also check the txpool for pending transactions
            try {
                const pending = await this.provider.send('txpool_content', []);
                if (pending && pending.pending) {
                    // Check pending transactions for this address
                    for (const [fromAddr, txsByNonce] of Object.entries(pending.pending)) {
                        if (fromAddr.toLowerCase() === address.toLowerCase()) {
                            for (const [nonce, tx] of Object.entries(txsByNonce)) {
                                // Check if we already have this transaction
                                const exists = txs.some(t => t.hash === tx.hash);
                                if (!exists) {
                                    txs.push({
                                        hash: tx.hash,
                                        from: tx.from,
                                        to: tx.to || 'Contract Creation',
                                        value: tx.value ? ethers.formatEther(tx.value) : '0',
                                        direction: 'out',
                                        blockNumber: 'pending',
                                        timestamp: null,
                                        status: 'pending',
                                        gasUsed: '—',
                                        gasPrice: tx.gasPrice ? ethers.formatEther(tx.gasPrice) : '—',
                                        nonce: parseInt(nonce)
                                    });
                                }
                            }
                        }
                    }
                }
            } catch (e) {
                console.warn('txpool_content not available:', e.message);
            }

            // Sort by block number (newest first)
            txs.sort((a, b) => {
                if (a.blockNumber === 'pending') return -1;
                if (b.blockNumber === 'pending') return 1;
                return b.blockNumber - a.blockNumber;
            });

            return txs;
        } catch (error) {
            console.error('Error getting transaction history:', error);
            return [];
        }
    }

    // === Get chain ID ===
    async getChainId() {
        try {
            const result = await this.provider.send('eth_chainId', []);
            return parseInt(result, 16);
        } catch (error) {
            console.warn('getChainId failed:', error.message);
            return 8979;
        }
    }

    // === TKM Domains and EmailVM ===
    async domainRpc(method, params = []) {
        return this.provider.send('tkmdomain_' + method, params);
    }

    async domainStatus() { return this.domainRpc('status'); }
    async domainSuperAddress() { return this.domainRpc('superAddress'); }
    async domainClaimSuper() { return this.domainRpc('claimSuper'); }
    async domainRegistrationFee() { return this.domainRpc('registrationFee'); }
    async domainSubscriberUnitPrice() { return this.domainRpc('subscriberUnitPrice'); }
    async domainQuote(totalUnits) { return this.domainRpc('quote', [ethers.toQuantity(totalUnits)]); }
    async domainOperator(totalUnits, amountTKM, domain, payout = '') {
        if (payout) return this.domainRpc('operatorWithPayout', [ethers.toQuantity(totalUnits), String(amountTKM), domain, payout]);
        return this.domainRpc('operator', [ethers.toQuantity(totalUnits), String(amountTKM), domain]);
    }
    async domainSetPayout(domain, payout) { return this.domainRpc('setPayout', [domain, payout]); }
    async domainBuy(username, domain) { return this.domainRpc('buy', [username, domain]); }
    async domainExpand(domain, additionalUnits, amountTKM) {
        return this.domainRpc('expand', [domain, ethers.toQuantity(additionalUnits), String(amountTKM)]);
    }
    async domainGet(domain) { return this.domainRpc('domain', [domain]); }
    async domainList() { return this.domainRpc('domains'); }
    async domainHash(domain) { return this.domainRpc('domainHash', [domain]); }
    async domainMailboxHash(username, domain) { return this.domainRpc('mailboxHash', [username, domain]); }
    async domainRegistration(registryHash) { return this.domainRpc('registration', [registryHash]); }
    async domainMailbox(mailbox) { return this.domainRpc('mailbox', [mailbox]); }
    async domainMailboxes(domain = '') { return this.domainRpc('mailboxes', [domain]); }
    async domainPending() { return this.domainRpc('pending'); }
    async domainSync() { return this.domainRpc('sync'); }

    async emailRpc(method, params = []) {
        return this.provider.send('emailvm_' + method, params);
    }

    async emailStatus() { return this.emailRpc('status'); }
    async emailPublishKey(mailbox, publicKey) { return this.emailRpc('publishKey', [mailbox, publicKey]); }
    async emailKey(mailbox) { return this.emailRpc('key', [mailbox]); }
    async emailSend(from, to, ciphertext, nonce) { return this.emailRpc('send', [from, to, ciphertext, nonce]); }
    async emailInbox(mailbox) { return this.emailRpc('inbox', [mailbox]); }
    async emailOutbox(mailbox) { return this.emailRpc('outbox', [mailbox]); }
    async emailInboxPage(mailbox, offset = 0, limit = 50) { return this.emailRpc('inboxPage', [mailbox, ethers.toQuantity(offset), ethers.toQuantity(limit)]); }
    async emailOutboxPage(mailbox, offset = 0, limit = 50) { return this.emailRpc('outboxPage', [mailbox, ethers.toQuantity(offset), ethers.toQuantity(limit)]); }
    async emailMessage(id) { return this.emailRpc('message', [id]); }

    // === TKM Phone RPC Methods ===
    async phoneRpc(method, params = []) {
        return this.provider.send('tkmphone_' + method, params);
    }

    async phoneStatus() { return this.phoneRpc('status'); }
    async phoneBuckets() { return this.phoneRpc('buckets'); }
    async phoneRegisteredNumbers() { return this.phoneRpc('registeredNumbers'); }
    async phoneListOperators() { return this.phoneRpc('listOperators'); }
    async phonePendingOperatorApprovals(scanBlocks = 20000) { return this.phoneRpc('pendingOperatorApprovals', [scanBlocks]); }
    async phoneNumber(number) { return this.phoneRpc('number', [number]); }
    async phoneDeviceKeys(number) { return this.phoneRpc('deviceKeys', [number]); }
    async phoneOpenBucketHash(operator, bucketId) { return this.phoneRpc('openBucketHash', [operator, bucketId]); }
    async phoneOpenBucket(operator, bucketId, signature) { return this.phoneRpc('openBucket', [operator, bucketId, signature]); }
    async phoneOperatorGrantHash(operator, keyHash, expiresAt, paymentTx) {
        return this.phoneRpc('operatorGrantHash', [operator, keyHash, expiresAt, paymentTx]);
    }
    async phoneRegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, paidWei, signature) {
        return this.phoneRpc('registerOperatorKey', [operator, keyHash, expiresAt, paymentTx, paidWei, signature]);
    }
    async phoneSellNumber(operator, number, buyer, priceWei, paymentTx) {
        return this.phoneRpc('sellNumber', [operator, number, buyer, priceWei, paymentTx]);
    }
    async phoneDeviceKeySigningHash(number, device, publicKey) {
        return this.phoneRpc('deviceKeySigningHash', [number, device, publicKey]);
    }
    async phoneDeviceKeySigningHashV2(number, device, publicKey, encryptionPublicKey) {
        return this.phoneRpc('deviceKeySigningHashV2', [number, device, publicKey, encryptionPublicKey]);
    }
    async phoneRegisterDeviceKey(number, device, publicKey, signature) {
        return this.phoneRpc('registerDeviceKey', [number, device, publicKey, signature]);
    }
    async phoneRegisterDeviceKeyV2(number, device, publicKey, encryptionPublicKey, signature) {
        return this.phoneRpc('registerDeviceKeyV2', [number, device, publicKey, encryptionPublicKey, signature]);
    }
    async phoneEncryptPayload(from, to, nonce, plaintext) {
        return this.phoneRpc('encryptPayload', [from, to, nonce, plaintext]);
    }
    async phoneDecryptPayload(from, to, nonce, ciphertext) {
        return this.phoneRpc('decryptPayload', [from, to, nonce, ciphertext]);
    }
    async phoneSendMessageSigningHash(from, to, nonce, ciphertext) {
        return this.phoneRpc('sendMessageSigningHash', [from, to, nonce, ciphertext]);
    }
    async phoneSendEncryptedMessage(from, to, ciphertext, nonce, signature) {
        return this.phoneRpc('sendEncryptedMessage', [from, to, ciphertext, nonce, signature]);
    }
    async phoneMessagesForNumber(number) {
        return this.phoneRpc('messagesForNumber', [number]);
    }
    async phoneNotifications(number) {
        return this.phoneRpc('notifications', [number]);
    }
    async phoneTransferNumberSigningHash(number, newOwner) {
        return this.phoneRpc('transferNumberSigningHash', [number, newOwner]);
    }
    async phoneTransferNumber(number, newOwner, signature) {
        return this.phoneRpc('transferNumber', [number, newOwner, signature]);
    }

}
