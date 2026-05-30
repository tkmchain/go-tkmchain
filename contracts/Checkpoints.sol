// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract Checkpoints {
    address public owner;
    bytes32 public checkpointHash;
    uint256 public setAtBlock;
    uint256 public setAtTimestamp;
    bool public isSet;

    constructor() {}

    function initialize(address _owner, bytes32 _hash) external {
        require(owner == address(0), "already initialized");
        owner = _owner;
        checkpointHash = _hash;
        setAtBlock = block.number;
        setAtTimestamp = block.timestamp;
        isSet = true;
    }

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    function setCheckpoint(bytes32 _hash) external onlyOwner {
        require(!isSet, "already set");
        checkpointHash = _hash;
        setAtBlock = block.number;
        setAtTimestamp = block.timestamp;
        isSet = true;
    }

    function getCheckpoint() external view returns (bytes32) {
        return checkpointHash;
    }
}
