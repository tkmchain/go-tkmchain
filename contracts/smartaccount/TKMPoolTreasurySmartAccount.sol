// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "./TKMSmartAccounts.sol";

/// @notice TKM mining-pool treasury account initialized to the public pool
/// wallet configured by the operator. It inherits all authorization, limits,
/// sessions, locking, and delayed recovery behavior from TKMAccount.
///
/// The initial owner is public configuration, not a secret. The corresponding
/// private key is never embedded in or supplied to this contract.
contract TKMPoolTreasurySmartAccount is TKMAccount {
    address public constant INITIAL_POOL_OWNER = 0x4441d6fEd0836B77a503e0B2788bfEd6FD8c23A8;
    string public constant ACCOUNT_PURPOSE = "TKM_MINING_POOL_TREASURY";
    uint256 public constant ACCOUNT_VERSION = 1;

    constructor(address entryPoint)
        TKMAccount(entryPoint, _initialOwners(), 1)
    {}

    function _initialOwners() private pure returns (address[] memory owners) {
        owners = new address[](1);
        owners[0] = INITIAL_POOL_OWNER;
    }
}
