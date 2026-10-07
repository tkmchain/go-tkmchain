# TKMNet Docker transit relays

This Compose project starts two onion-published TKMNet transit relays. Each has
its own Tor v3 hidden-service identity, ML-KEM relay key, ML-DSA descriptor
signing seed, and persistent state directory. The TKMNet listener is bound to
container loopback; Docker publishes no clearnet port.

Build the relay utility with the repository's pinned Go toolchain, build its
runtime image once from the repository root, then start from this directory:

```sh
mkdir -p deploy/tkmnet-relays/bin
GOTOOLCHAIN=auto CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
  -o deploy/tkmnet-relays/bin/tkmnet-relay ./cmd/tkmnet-relay
sudo docker build -f deploy/tkmnet-relays/Dockerfile -t local/tkmnet-relay:dev .
cd deploy/tkmnet-relays
sudo docker compose up -d
sudo docker compose ps
sudo docker compose logs -f
```

The signed public descriptors are written to `state/relay1/descriptor.json`
and `state/relay2/descriptor.json`; onion hostnames are in
`state/tor1/hidden/hostname` and `state/tor2/hidden/hostname`. Keep each data
and Tor state directory persistent and protected. The relay service's private
KEM key and descriptor-signing seed are private material.

`healthz` means the local relay process is listening. `readyz` remains
unavailable until `config/relay*-peers.json` contains verified, current
signed descriptors for the next relays and directory endpoints. This is
intentional: an unconfigured listener is not advertised as a working route.
Descriptors expire within 24 hours and must be renewed and redistributed
before expiry. These two containers share one host and are suitable for
integration testing, not independent production operators.
