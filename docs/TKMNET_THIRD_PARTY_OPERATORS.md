# Paid third-party TKMNet operators

This guide is for providers who will host TKMNet services for TKMChain. The
minimum production deployment has **four independently operated services**:

1. Two username-directory operators.
2. Two transit relays, operated separately from both directory operators and
   from each other.

The node needs all four peers configured. The username lookup builds a
three-hop route: two distinct transit relays followed by the selected
directory. Its validators require distinct relay keys and onion services, and
reject either transit relay if it is also a directory operator. Two directory
operators are also required so the queried username and its cover alias can be
assigned to separate operators.

## Paid service scope

The directory operators retain and serve signed username bindings. A binding
includes the username, chain ID, Shield3 address, full Shield3 payment code,
owner signature, sequence, and lease expiry. Treat directory storage as
sensitive personal/payment data: restrict staff access, encrypt storage and
backups, patch promptly, and publish a retention and incident contact policy.
Directory records are replicated application data, not consensus state.

Transit relays forward fixed-size encrypted TKMNet packets over onion services.
They do not need directory database access. Their relay keys and operator
administration must be separate from the directory services. A directory
operator can see the username queries assigned to it; colluding directory
operators can correlate activity. TKMNet does not provide cryptographic PIR.

The provider agreement should set the setup fee, recurring fee, payment method,
support contact, uptime target, maintenance notice, abuse process, backup and
retention policy, incident notification window, and termination/key-rotation
procedure. These are commercial payments to providers. The current protocol
does not issue an on-chain operator reward or automatically pay these invoices.
Do not send provider payments to a consensus or main-king address.

## Provider requirements

Each service must have its own v3 onion hostname, private signing/relay keys,
administrator account, and host or separately administered VM. For the two
transit relays, use different providers or independently controlled networks
and regions where practical. Do not count aliases, containers, or two keys on
one host as independent operators. Keep the relay listener bound to loopback;
Tor publishes it. Do not open the relay port on the public firewall.

Each provider must deliver through the agreed authenticated channel:

- Service role: `directory-1`, `directory-2`, `transit-1`, or `transit-2`.
- v3 `.onion` hostname and relay port.
- A current, valid TKMNet signed descriptor, including its relay ID,
  ML-KEM-1024 public key, ML-DSA-87 signing public key, expiry, and signature.
- For each directory only, the SHA-256 fingerprint of the descriptor's
  ML-DSA-87 signing public key (`SigningKeyPin`).
- Renewal/expiry contact and planned key-rotation procedure.
- A live Tor reachability and TKMNet health test agreed with TKMChain.

The descriptor signature authenticates the descriptor; the independently
verified key pin says which directory signer TKMChain trusts. Never accept a
pin sent only by the operator whose key it pins. Verify pins out of band with
TKMChain and verify that all four onion hosts, descriptor IDs, and keys are
distinct. Descriptors expire, so the provider must arrange renewal before
expiry and TKMChain must distribute updated descriptors and pins.

## Node configuration

Use a `gtkm` release that supports username networking and TKMNet. Install and
run Tor on the node; the example below uses Tor's local SOCKS5 listener on
`127.0.0.1:9050`. Configure Tor hidden services so each provider's onion port
forwards to that node's loopback listener, normally `127.0.0.1:39000`.

In the node's TOML configuration, set the `[Tkmnet]` section. The snippet
below configures the local transport only; it is intentionally incomplete and
will **not** pass username-network readiness until the four peer descriptors
and two verified pins are installed by a supported import path. Do not copy it
as a complete production configuration:

```toml
[Tkmnet]
Enabled = true
ListenAddr = "127.0.0.1:39000"
OnionOnly = true
HopIndex = 0
PrivateKeyPath = ""
SOCKS5Proxy = "socks5://127.0.0.1:9050"
RelayPort = "39000"

# TransitPeers: signed descriptors for transit-1 and transit-2.
# DirectoryPeers: signed descriptors and verified SigningKeyPin values for
# directory-1 and directory-2.
# RelayPeers: forwarding allow-list containing all four signed descriptors.
```

The TOML decoder expects full descriptor data, not just an onion hostname.
`PrivateKeyPath = ""` uses the node's default relay-key path under its data
directory. Keep that private key file mode `0600` and include it in protected
backups. Keep descriptor expiries current. A node may not start or report the
username network ready when descriptors, pins, Tor, or peer health checks are
invalid or unavailable.

Start `gtkm` with onion-only P2P settings and TKMNet enabled, for example:

```sh
./build/bin/gtkm \
  --privacy.onion-only \
  --p2p.tor-socks5=socks5://127.0.0.1:9050 \
  --p2p.onion-hostname=<this-node>.onion \
  --tkmnet.enable \
  --tkmnet.listen=127.0.0.1:39000 \
  --config="$HOME/.tkmchain/gtkm/config.toml"
```

Use the node's normal data directory, RPC restrictions, bootnodes, and service
manager options as well. Do not expose RPC publicly to make TKMNet work. After
the username-network activation, TKMNet is enabled from the canonical chain
head automatically; the peer and Tor configuration is still required.

## Important release limitation

The current source documents and validates signed descriptors and pins, but it
does **not** include a supported descriptor generation/export command or
operator onboarding UI. Do not invent TOML keys, hand-edit public-key arrays,
or declare a provider online based only on a running onion service. Before
production cutover, TKMChain must provide a supported descriptor import/export
path, securely verify the four bundles and two directory pins, then run the
readiness and username replication checks on the target node. Until that is
done, this document is a provider specification and deployment checklist; it
does not mean the four paid services are already configured or network-ready.

## Readiness checks

After installing real descriptor bundles and pins:

1. Confirm Tor can reach every `.onion` endpoint from the node through the
   configured SOCKS5 proxy.
2. Confirm all four descriptors verify and have distinct IDs, signing keys,
   and onion hosts; verify the directory pins independently.
3. Confirm the node has two entries in `TransitPeers`, two in
   `DirectoryPeers`, and all four peers in `RelayPeers`.
4. Check `tkmname` network status and require `networkReady` only after the
   directory health probes and initial binding publication succeed.
5. Register a test username, verify both directory operators acknowledge it,
   resolve it through separate transit circuits, then rotate one descriptor
   and verify expiry/renewal handling.
6. Recheck readiness after node restart and after a simulated relay outage.

Never advertise network readiness from a local database-only lookup. The
operator set and relay paths must pass the network checks.
