#!/usr/bin/env python3
"""Deploy the deterministic C++ TVM fixture and verify stored code visibility."""

import argparse
import json
from pathlib import Path
import sys
import time
import urllib.request


MODULE_RETURN_CODE_HASH = "0x01"
METADATA = {
    "name": "TestReturnCodeHash",
    "language": "cpp",
    "source": "contracts/tvm/test_return_code_hash.cpp",
    "target": "cpp-evm-v1",
}


def rpc(url, method, params=None):
    payload = json.dumps({
        "jsonrpc": "2.0",
        "method": method,
        "params": params or [],
        "id": int(time.time() * 1000),
    }).encode()
    req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as res:
        data = json.loads(res.read().decode())
    if "error" in data:
        raise RuntimeError(f"{method}: {data['error'].get('message', data['error'])}")
    return data.get("result")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--rpc", default="http://127.0.0.1:8545", help="GTKM JSON-RPC URL")
    parser.add_argument("--from", dest="sender", help="deployer address")
    parser.add_argument("--pool-config", help="pool config.json containing poolWallet and poolWalletPassword")
    parser.add_argument("--passphrase-file", help="file containing the deployer account passphrase")
    parser.add_argument("--gas", default="0x100000", help="deployment gas limit")
    parser.add_argument("--poll-seconds", type=int, default=120, help="receipt polling timeout")
    args = parser.parse_args()

    passphrase = None
    if args.pool_config:
        pool_config = json.loads(Path(args.pool_config).read_text())
        args.sender = args.sender or pool_config.get("poolWallet")
        passphrase = pool_config.get("poolWalletPassword")
    if args.passphrase_file:
        passphrase = Path(args.passphrase_file).read_text().splitlines()[0]
    if not args.sender:
        parser.error("--from is required unless --pool-config provides poolWallet")

    metadata_hex = "0x" + json.dumps(METADATA, separators=(",", ":")).encode().hex()
    build = rpc(args.rpc, "tvm_buildDeployment", [{
        "code": MODULE_RETURN_CODE_HASH,
        "metadata": metadata_hex,
        "memoryPages": 1,
        "stackSlots": 16,
        "callDepth": 4,
    }])
    deployment_code = build["deploymentCode"]
    tx_args = {
        "from": args.sender,
        "data": deployment_code,
        "gas": args.gas,
    }
    if passphrase:
        tx_hash = rpc(args.rpc, "tkm_sendTransactionWithPassphrase", [tx_args, passphrase])
    else:
        tx_hash = rpc(args.rpc, "eth_sendTransaction", [tx_args])
    print(json.dumps({"transactionHash": tx_hash, "status": "submitted"}, indent=2), flush=True)

    deadline = time.time() + args.poll_seconds
    receipt = None
    while time.time() < deadline:
        receipt = rpc(args.rpc, "eth_getTransactionReceipt", [tx_hash])
        if receipt:
            break
        time.sleep(2)
    if not receipt:
        pending = rpc(args.rpc, "eth_getTransactionByHash", [tx_hash])
        latest = rpc(args.rpc, "eth_blockNumber", [])
        raise RuntimeError(
            "deployment transaction was not mined before timeout: "
            f"{tx_hash}; latestBlock={latest}; pending={pending is not None and pending.get('blockNumber') is None}"
        )
    if receipt.get("status") not in ("0x1", 1, True, None):
        raise RuntimeError(f"deployment failed: {json.dumps(receipt, indent=2)}")

    contract = receipt.get("contractAddress")
    if not contract:
        raise RuntimeError(f"receipt did not include contractAddress: {json.dumps(receipt, indent=2)}")
    stored = rpc(args.rpc, "eth_getCode", [contract, "latest"])
    decoded = rpc(args.rpc, "tvm_getCode", [contract, "latest"])
    if stored.lower() != deployment_code.lower():
        raise RuntimeError("eth_getCode did not return the full deployed TVM envelope")
    if decoded.get("envelope", "").lower() != deployment_code.lower():
        raise RuntimeError("tvm_getCode did not return the full deployed TVM envelope")

    print(json.dumps({
        "transactionHash": tx_hash,
        "contractAddress": contract,
        "blockNumber": receipt.get("blockNumber"),
        "codeHash": decoded.get("codeHash"),
        "metadataHash": decoded.get("metadataHash"),
        "target": decoded.get("target"),
        "storedCodeBytes": (len(stored) - 2) // 2,
    }, indent=2))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
