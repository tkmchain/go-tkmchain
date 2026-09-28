// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title TKMChain append-only block-hash anchors
/// @notice Stores a contiguous, immutable sequence of canonical block hashes.
///
/// The EVM's BLOCKHASH opcode can only prove hashes for the previous 256
/// blocks. This contract deliberately refuses older hashes instead of
/// accepting an owner-supplied value that it cannot verify. Deploy it near the
/// beginning of a network, or use a chain-native historical-hash oracle before
/// trying to backfill older history.
///
/// Anchors are application-level evidence. They do not replace TKMChain's
/// consensus checkpoints or make a reorganization impossible: a reorg that
/// removes the transaction that wrote an anchor also removes that contract
/// state. Nodes must compare the events with their canonical chain and enforce
/// their configured consensus checkpoints.
contract TKMBlockHashAnchors {
    error InvalidOwner();
    error NotOwner();
    error FutureHeight(uint64 height, uint256 currentHeight);
    error HistoricalHashUnavailable(uint64 height);
    error EmptyHash();
    error HashMismatch(uint64 height, bytes32 expectedHash, bytes32 canonicalHash);
    error AlreadyAnchored(uint64 height);
    error NonContiguousHeight(uint64 expected, uint64 supplied);
    error RangeTooLarge(uint256 length);
    error HeightOverflow();

    address public owner;
    bool public initialized;
    uint64 public firstHeight;
    uint64 public latestHeight;
    uint64 public anchorCount;
    bytes32 public rollingCommitment;

    bytes32 public constant ANCHOR_DOMAIN = keccak256("TKMCHAIN_BLOCK_HASH_ANCHOR_V1");

    mapping(uint64 => bytes32) private _anchors;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event BlockHashAnchored(
        uint64 indexed height,
        bytes32 indexed blockHash,
        address indexed submitter,
        bytes32 rollingCommitment
    );

    modifier onlyOwner() {
        if (msg.sender != owner) revert NotOwner();
        _;
    }

    constructor(address initialOwner) {
        if (initialOwner == address(0)) revert InvalidOwner();
        owner = initialOwner;
        emit OwnershipTransferred(address(0), initialOwner);
    }

    /// @notice Transfer the append authority. Existing anchors are unaffected.
    function transferOwnership(address newOwner) external onlyOwner {
        if (newOwner == address(0)) revert InvalidOwner();
        address previousOwner = owner;
        owner = newOwner;
        emit OwnershipTransferred(previousOwner, newOwner);
    }

    /// @notice Anchor one recent block using the hash supplied by the caller.
    /// @dev The value is checked against the canonical BLOCKHASH opcode before
    /// it is written. Heights must be contiguous after the first anchor.
    function appendVerified(uint64 height, bytes32 expectedHash) external onlyOwner {
        bytes32 canonicalHash = _canonicalHash(height);
        if (canonicalHash != expectedHash) revert HashMismatch(height, expectedHash, canonicalHash);
        _append(height, canonicalHash);
    }

    /// @notice Anchor one recent block without requiring the caller to copy its
    /// hash. The contract obtains and verifies the canonical value itself.
    function appendCanonical(uint64 height) external onlyOwner returns (bytes32 blockHash) {
        blockHash = _canonicalHash(height);
        _append(height, blockHash);
    }

    /// @notice Anchor the parent of the block containing this transaction.
    /// This is convenient for an operator that submits one anchor per block.
    function appendParent() external onlyOwner returns (uint64 height, bytes32 blockHash) {
        if (block.number == 0 || block.number - 1 > type(uint64).max) revert HeightOverflow();
        height = uint64(block.number - 1);
        blockHash = _canonicalHash(height);
        _append(height, blockHash);
    }

    /// @notice Append a contiguous recent range. Every element is checked with
    /// BLOCKHASH before any state is changed.
    function appendVerifiedRange(uint64 startHeight, bytes32[] calldata expectedHashes) external onlyOwner {
        if (expectedHashes.length == 0 || expectedHashes.length > 256) {
            revert RangeTooLarge(expectedHashes.length);
        }
        uint256 last = uint256(startHeight) + expectedHashes.length - 1;
        if (last > type(uint64).max) revert HeightOverflow();

        // Validate the complete range first so a bad later element cannot
        // leave a partially appended sequence in the transaction state.
        for (uint256 i = 0; i < expectedHashes.length; i++) {
            uint64 height = uint64(uint256(startHeight) + i);
            bytes32 canonicalHash = _canonicalHash(height);
            if (canonicalHash != expectedHashes[i]) revert HashMismatch(height, expectedHashes[i], canonicalHash);
        }
        for (uint256 i = 0; i < expectedHashes.length; i++) {
            _append(uint64(uint256(startHeight) + i), expectedHashes[i]);
        }
    }

    /// @notice Return an anchored hash and whether that height has been stored.
    function anchorAt(uint64 height) external view returns (bytes32 blockHash, bool anchored) {
        blockHash = _anchors[height];
        anchored = blockHash != bytes32(0);
    }

    /// @notice Check an anchored hash, or check the live BLOCKHASH window when
    /// the height has not been stored yet.
    function matchesCanonical(uint64 height, bytes32 expectedHash) external view returns (bool) {
        if (expectedHash == bytes32(0)) return false;
        bytes32 anchoredHash = _anchors[height];
        if (anchoredHash != bytes32(0)) return anchoredHash == expectedHash;
        if (height >= block.number || block.number - uint256(height) > 256) return false;
        return blockhash(uint256(height)) == expectedHash;
    }

    function _canonicalHash(uint64 height) private view returns (bytes32 blockHash) {
        if (height >= block.number) revert FutureHeight(height, block.number);
        if (block.number - uint256(height) > 256) revert HistoricalHashUnavailable(height);
        blockHash = blockhash(uint256(height));
        if (blockHash == bytes32(0)) revert EmptyHash();
    }

    function _append(uint64 height, bytes32 blockHash) private {
        if (blockHash == bytes32(0)) revert EmptyHash();
        if (_anchors[height] != bytes32(0)) revert AlreadyAnchored(height);
        if (!initialized) {
            initialized = true;
            firstHeight = height;
        } else {
            if (latestHeight == type(uint64).max) revert HeightOverflow();
            uint64 expectedHeight = latestHeight + 1;
            if (height != expectedHeight) revert NonContiguousHeight(expectedHeight, height);
        }
        _anchors[height] = blockHash;
        latestHeight = height;
        anchorCount += 1;
        rollingCommitment = keccak256(abi.encode(ANCHOR_DOMAIN, rollingCommitment, height, blockHash));
        emit BlockHashAnchored(height, blockHash, msg.sender, rollingCommitment);
    }
}
