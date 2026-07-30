// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @notice A deliberately small account-abstraction implementation for TKMChain.
/// It uses ordinary contract calls and therefore requires no consensus change.
library TKMSA {
    struct UserOperation {
        address account;
        address target;
        uint256 value;
        bytes data;
        uint256 nonce;
        uint48 validUntil;
        uint256 gasLimit;
        address paymaster;
        bytes paymasterData;
    }

    function hash(UserOperation calldata op, address entryPoint) internal view returns (bytes32) {
        return keccak256(abi.encode(
            keccak256("TKMUserOperation(address account,address target,uint256 value,bytes32 dataHash,uint256 nonce,uint48 validUntil,uint256 gasLimit,address paymaster,bytes32 paymasterDataHash,address entryPoint,uint256 chainId)"),
            op.account, op.target, op.value, keccak256(op.data), op.nonce, op.validUntil,
            op.gasLimit, op.paymaster, keccak256(op.paymasterData), entryPoint, block.chainid
        ));
    }

    function ethHash(bytes32 digest) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", digest));
    }

    function recover(bytes32 digest, bytes memory signature) internal pure returns (address) {
        if (signature.length != 65) return address(0);
        bytes32 r; bytes32 s; uint8 v;
        assembly { r := mload(add(signature, 32)) s := mload(add(signature, 64)) v := byte(0, mload(add(signature, 96))) }
        if (v < 27) v += 27;
        if (v != 27 && v != 28) return address(0);
        if (uint256(s) > 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0) return address(0);
        return ecrecover(ethHash(digest), v, r, s);
    }
}

interface ITKMAccount {
    function validateUserOperation(TKMSA.UserOperation calldata op, bytes32 opHash, bytes calldata signature) external returns (bool);
    function executeFromEntryPoint(address target, uint256 value, bytes calldata data) external returns (bytes memory);
}

interface ITKMPaymaster {
    function validateSponsorship(TKMSA.UserOperation calldata op, bytes32 opHash) external view returns (bool);
}

contract TKMEntryPoint {
    mapping(address => uint256) public nonces;
    bool private entered;

    event UserOperationHandled(bytes32 indexed operationHash, address indexed account, address indexed target, address paymaster, bool success, bytes result);

    error Reentrant();
    error InvalidAccount();
    error InvalidNonce();
    error Expired();
    error InvalidAuthorization();
    error InvalidSponsorship();

    function operationHash(TKMSA.UserOperation calldata op) external view returns (bytes32) { return TKMSA.hash(op, address(this)); }

    function handleOperation(TKMSA.UserOperation calldata op, bytes calldata signature) external returns (bytes memory result) {
        if (entered) revert Reentrant();
        entered = true;
        if (op.account.code.length == 0) revert InvalidAccount();
        if (op.nonce != nonces[op.account]) revert InvalidNonce();
        if (op.validUntil != 0 && block.timestamp > op.validUntil) revert Expired();
        bytes32 digest = TKMSA.hash(op, address(this));
        if (!ITKMAccount(op.account).validateUserOperation(op, digest, signature)) revert InvalidAuthorization();
        if (op.paymaster != address(0) && !ITKMPaymaster(op.paymaster).validateSponsorship(op, digest)) revert InvalidSponsorship();
        nonces[op.account]++;
        result = ITKMAccount(op.account).executeFromEntryPoint{gas: op.gasLimit}(op.target, op.value, op.data);
        entered = false;
        emit UserOperationHandled(digest, op.account, op.target, op.paymaster, true, result);
    }
}

contract TKMAccount {
    using TKMSA for bytes32;

    address public immutable entryPoint;
    mapping(address => bool) public owners;
    address[] private ownerList;
    uint16 public ownerCount;
    uint16 public threshold;
    uint256 public highValueThreshold;
    uint256 public dailyLimit;
    uint256 public spentToday;
    uint64 public spendingDay;
    bool public locked;

    mapping(address => bool) public guardians;
    uint16 public guardianCount;
    uint16 public guardianThreshold;
    uint48 public recoveryDelay;
    uint256 public recoveryNonce;

    struct Recovery { bytes32 ownerHash; uint48 executeAfter; uint16 approvals; bool active; }
    Recovery public recovery;
    mapping(uint256 => mapping(address => bool)) public recoveryApproved;

    struct Session {
        address target;
        bytes4 selector;
        uint128 maxValuePerCall;
        uint128 remainingValue;
        uint48 validAfter;
        uint48 validUntil;
        bool active;
    }
    mapping(address => Session) public sessions;

    event Executed(address indexed target, uint256 value, bytes4 selector);
    event OwnersChanged(address[] owners, uint16 threshold);
    event GuardianChanged(address indexed guardian, bool enabled);
    event RecoveryStarted(bytes32 indexed ownerHash, uint48 executeAfter, uint256 nonce);
    event RecoveryCancelled(uint256 nonce);
    event RecoveryCompleted(uint256 nonce);
    event SessionChanged(address indexed key, bool enabled);
    event Locked(bool locked);

    modifier onlySelf() { require(msg.sender == address(this), "self only"); _; }
    modifier onlyEntryPoint() { require(msg.sender == entryPoint, "entry point only"); _; }

    constructor(address entryPoint_, address[] memory owners_, uint16 threshold_) {
        require(entryPoint_ != address(0), "zero entry point");
        entryPoint = entryPoint_;
        recoveryDelay = 48 hours;
        dailyLimit = type(uint256).max;
        highValueThreshold = type(uint256).max;
        _setOwners(owners_, threshold_);
    }

    receive() external payable {}

    function validateUserOperation(TKMSA.UserOperation calldata op, bytes32 opHash, bytes calldata authorization) external onlyEntryPoint returns (bool) {
        if (locked || op.account != address(this) || op.target == address(0)) return false;
        if (authorization.length < 1) return false;
        uint8 mode = uint8(authorization[0]);
        bytes memory payload = authorization[1:];
        if (mode == 1) {
            address key = TKMSA.recover(opHash, payload);
            Session storage session = sessions[key];
            bytes4 selector = op.data.length >= 4 ? bytes4(op.data[:4]) : bytes4(0);
            bool validSession = session.active && session.target == op.target && session.selector == selector &&
                block.timestamp >= session.validAfter && block.timestamp <= session.validUntil &&
                op.value <= session.maxValuePerCall && op.value <= session.remainingValue;
            if (validSession) session.remainingValue -= uint128(op.value);
            return validSession;
        }
        if (mode == 2) {
            bytes[] memory signatures = abi.decode(payload, (bytes[]));
            uint256 required = op.value >= highValueThreshold ? threshold : 1;
            uint256 valid;
            address previous;
            for (uint256 i; i < signatures.length; i++) {
                address signer = TKMSA.recover(opHash, signatures[i]);
                if (owners[signer] && signer > previous) { valid++; previous = signer; }
            }
            return valid >= required;
        }
        return false;
    }

    function executeFromEntryPoint(address target, uint256 value, bytes calldata data) external onlyEntryPoint returns (bytes memory result) {
        uint64 day = uint64(block.timestamp / 1 days);
        if (day != spendingDay) { spendingDay = day; spentToday = 0; }
        require(value <= dailyLimit - spentToday, "daily limit");
        spentToday += value;
        (bool ok, bytes memory out) = target.call{value: value}(data);
        if (!ok) assembly { revert(add(out, 32), mload(out)) }
        emit Executed(target, value, data.length >= 4 ? bytes4(data[:4]) : bytes4(0));
        return out;
    }

    function setOwners(address[] calldata newOwners, uint16 newThreshold) external onlySelf { _setOwners(newOwners, newThreshold); }
    function _setOwners(address[] memory newOwners, uint16 newThreshold) internal {
        require(newOwners.length > 0 && newOwners.length <= 32, "owner count");
        for (uint256 i; i < ownerList.length; i++) owners[ownerList[i]] = false;
        delete ownerList;
        require(newThreshold > 0 && newThreshold <= newOwners.length, "threshold");
        for (uint256 i; i < newOwners.length; i++) {
            require(newOwners[i] != address(0) && !owners[newOwners[i]], "invalid owner");
            ownerList.push(newOwners[i]);
            owners[newOwners[i]] = true;
        }
        ownerCount = uint16(newOwners.length); threshold = newThreshold;
        emit OwnersChanged(newOwners, newThreshold);
    }

    function setLimits(uint256 daily, uint256 highValue) external onlySelf { dailyLimit = daily; highValueThreshold = highValue; }
    function setLocked(bool value) external onlySelf { locked = value; emit Locked(value); }

    function setGuardian(address guardian, bool enabled) external onlySelf {
        require(guardian != address(0), "zero guardian");
        if (guardians[guardian] != enabled) { guardians[guardian] = enabled; enabled ? guardianCount++ : guardianCount--; }
        emit GuardianChanged(guardian, enabled);
    }
    function setRecoveryPolicy(uint16 required, uint48 delay) external onlySelf {
        require(required > 0 && required <= guardianCount, "guardian threshold");
        require(delay >= 1 hours && delay <= 30 days, "recovery delay");
        guardianThreshold = required; recoveryDelay = delay;
    }
    function approveRecovery(bytes32 newOwnerHash) external {
        require(guardians[msg.sender] && guardianThreshold > 0, "guardian only");
        if (!recovery.active || recovery.ownerHash != newOwnerHash) {
            recoveryNonce++;
            recovery = Recovery(newOwnerHash, uint48(block.timestamp) + recoveryDelay, 0, true);
        }
        require(!recoveryApproved[recoveryNonce][msg.sender], "already approved");
        recoveryApproved[recoveryNonce][msg.sender] = true; recovery.approvals++;
        emit RecoveryStarted(newOwnerHash, recovery.executeAfter, recoveryNonce);
    }
    function cancelRecovery() external onlySelf { recovery.active = false; recoveryNonce++; emit RecoveryCancelled(recoveryNonce); }
    function completeRecovery(address[] calldata newOwners, uint16 newThreshold) external {
        require(recovery.active && recovery.approvals >= guardianThreshold && block.timestamp >= recovery.executeAfter, "recovery unavailable");
        require(keccak256(abi.encode(newOwners, newThreshold)) == recovery.ownerHash, "owner hash");
        recovery.active = false; _setOwners(newOwners, newThreshold); locked = false;
        emit RecoveryCompleted(recoveryNonce);
    }

    function setSession(address key, Session calldata session) external onlySelf {
        require(key != address(0) && session.target != address(0), "invalid session");
        require(session.validUntil > session.validAfter, "invalid validity");
        sessions[key] = session; emit SessionChanged(key, session.active);
    }
    function revokeSession(address key) external onlySelf { delete sessions[key]; emit SessionChanged(key, false); }
}

contract TKMAccountFactory {
    address public immutable entryPoint;
    event AccountCreated(address indexed account, bytes32 indexed salt);
    constructor(address entryPoint_) { require(entryPoint_ != address(0), "zero entry point"); entryPoint = entryPoint_; }
    function createAccount(address[] calldata owners, uint16 threshold, bytes32 salt) external returns (address account) {
        account = address(new TKMAccount{salt: salt}(entryPoint, owners, threshold));
        emit AccountCreated(account, salt);
    }
    function predictAccount(address[] calldata owners, uint16 threshold, bytes32 salt) external view returns (address) {
        bytes32 codeHash = keccak256(abi.encodePacked(type(TKMAccount).creationCode, abi.encode(entryPoint, owners, threshold)));
        return address(uint160(uint256(keccak256(abi.encodePacked(bytes1(0xff), address(this), salt, codeHash)))));
    }
}

contract TKMAllowlistPaymaster {
    address public owner;
    address public signer;
    mapping(address => mapping(bytes4 => bool)) public allowed;
    bool public paused;
    event Allowed(address indexed target, bytes4 indexed selector, bool enabled);
    constructor(address signer_) { owner = msg.sender; signer = signer_; }
    modifier onlyOwner() { require(msg.sender == owner, "owner only"); _; }
    function setAllowed(address target, bytes4 selector, bool enabled) external onlyOwner { allowed[target][selector] = enabled; emit Allowed(target, selector, enabled); }
    function setSigner(address value) external onlyOwner { require(value != address(0), "zero signer"); signer = value; }
    function setPaused(bool value) external onlyOwner { paused = value; }
    function transferOwnership(address value) external onlyOwner { require(value != address(0), "zero owner"); owner = value; }
    function validateSponsorship(TKMSA.UserOperation calldata op, bytes32 opHash) external view returns (bool) {
        if (paused || op.paymaster != address(this) || op.data.length < 4) return false;
        bytes4 selector = bytes4(op.data[:4]);
        if (!allowed[op.target][selector]) return false;
        (uint48 expiry, bytes memory signature) = abi.decode(op.paymasterData, (uint48, bytes));
        if (expiry < block.timestamp || expiry > block.timestamp + 1 days) return false;
        bytes32 permit = keccak256(abi.encode("TKM_SPONSOR_V1", opHash, expiry, address(this), block.chainid));
        return TKMSA.recover(permit, signature) == signer;
    }
}
