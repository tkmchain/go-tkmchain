// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title TKM-native asset identity
/// @notice Optional contract-facing interface for the immutable TKM manifest.
/// The chain-level classifier still verifies the runtime-code trailer, so a
/// contract must not treat these methods alone as proof of native status.
interface ITKMAsset {
    function tkmAssetManifest() external view returns (bytes memory);
    function tkmAssetId() external view returns (bytes32);
    function tkmAssetKind() external view returns (uint8);
    function tkmAssetPolicyHash() external view returns (bytes32);
}

/// @notice Calls the TKM Antartical asset-ID primitive at 0x...f3.
library TKMAssetIdentity {
    address internal constant PRECOMPILE = 0x00000000000000000000000000000000000000f3;

    function compute(bytes32 chainId, address contractAddress, uint8 kind, bytes32 manifestHash) internal view returns (bytes32 assetId) {
        bytes memory input = abi.encodePacked(chainId, contractAddress, kind, manifestHash);
        (bool ok, bytes memory output) = PRECOMPILE.staticcall(input);
        require(ok && output.length == 32, "TKM asset identity failed");
        assembly {
            assetId := mload(add(output, 32))
        }
    }
}

