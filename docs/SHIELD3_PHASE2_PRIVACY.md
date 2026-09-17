# Shield3 Phase 2 network privacy

Phase 2 hardens the path a private Shield3 transaction takes across the network. It is active only when the node's existing Antartical fork rules are active; the transaction envelope and consensus validation are unchanged.

## Wallet-to-relay transport

Wallet relay HTTP calls now support an explicit SOCKS5 proxy, normally a local Tor listener:

```go
cfg := shield3wallet.RelayTransportConfig{
    SOCKS5Proxy:       "socks5://127.0.0.1:9050",
    FixedRequestBytes: shield3wallet.DefaultRelayRequestBytes,
    RequestDelay:      2 * time.Second,
}
offer, endpoint, err := shield3wallet.FetchRelayOfferAny(ctx, endpoints, requestID, cfg)
```

The transport disables ambient `HTTP_PROXY` and `HTTPS_PROXY` settings. `.onion` relay URLs are rejected unless an explicit SOCKS5 proxy is configured. Remote relays still require HTTPS. Relay request and success response bodies are padded to a fixed 32 KiB by default. Padding is bounded and ignored by the protocol.

`FetchRelayOfferAny` tries relay endpoints one at a time. This matters because an offer reserves an operator nonce; probing all relays concurrently would create unnecessary leases. Once a wallet has built a payment for one offer, it must submit it to that same endpoint. The wallet must never submit the same payer draft to another operator after a submission may have succeeded.

## Dandelion-style propagation

After Antartical, Shield3 envelopes are sent to one deterministic stem peer first. After a three-second bounded delay, the node releases the transaction through the normal peer fanout. Ordinary transactions retain the existing propagation behavior. This reduces immediate sender-to-many-peer correlation without changing signed bytes or block validity.

The stem path is best-effort network privacy. Transactions released during one delay window are sent as a batch, while the existing peer broadcaster caps each wire packet. It does not hide the canonical transaction hash from validators, and it cannot protect against a malicious or colluding majority of peers.

## What remains public

Consensus still exposes the canonical transaction hash, block inclusion, gas fields, commitments/nullifiers required by the current Shield3 verifier, and the relay operator for sponsored transactions. Deposit funding remains public. Phase 2 hides the wallet's direct network path and reduces timing/size classification; it does not make a public hash disappear.

## Operational requirements

- Run Tor locally and configure a SOCKS5 listener before using `.onion` relays.
- Configure several independently operated relays and use `FetchRelayOfferAny` for offer acquisition.
- Keep the request ID and signed relay bytes durable so retries use the exact same transaction.
- Treat this as defense in depth. An independent cryptographic and network audit remains required before relying on the Antartical hardfork for high-value privacy.
