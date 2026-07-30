package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

const smartAccountVersion = "1.0.0"

var smartAccountTypeHash = crypto.Keccak256Hash([]byte("TKMUserOperation(address account,address target,uint256 value,bytes32 dataHash,uint256 nonce,uint48 validUntil,uint256 gasLimit,address paymaster,bytes32 paymasterDataHash,address entryPoint,uint256 chainId)"))

// SmartAccountAPI builds and validates calls for the non-consensus smart-account
// contracts. It never unlocks accounts, signs operations, or changes chain state.
type SmartAccountAPI struct{}

func NewSmartAccountAPI() *SmartAccountAPI { return &SmartAccountAPI{} }

type SmartAccountStatus struct {
	Version      string           `json:"version"`
	Consensus    bool             `json:"consensus"`
	RequiresFork bool             `json:"requiresHardfork"`
	Deployed     bool             `json:"deployed"`
	EntryPoint   common.Address   `json:"entryPoint"`
	Factory      common.Address   `json:"factory"`
	Paymaster    common.Address   `json:"paymaster"`
	AuthModes    map[string]uint8 `json:"authorizationModes"`
}

type SmartUserOperation struct {
	Account       common.Address `json:"account"`
	Target        common.Address `json:"target"`
	Value         *hexutil.Big   `json:"value"`
	Data          hexutil.Bytes  `json:"data"`
	Nonce         *hexutil.Big   `json:"nonce"`
	ValidUntil    hexutil.Uint64 `json:"validUntil"`
	GasLimit      *hexutil.Big   `json:"gasLimit"`
	Paymaster     common.Address `json:"paymaster"`
	PaymasterData hexutil.Bytes  `json:"paymasterData"`
}

type SmartAccountCreateRequest struct {
	Owners    []common.Address `json:"owners"`
	Threshold uint16           `json:"threshold"`
	Salt      common.Hash      `json:"salt"`
}

type SmartAccountSessionRequest struct {
	Key             common.Address `json:"key"`
	Target          common.Address `json:"target"`
	Selector        hexutil.Bytes  `json:"selector"`
	MaxValuePerCall *hexutil.Big   `json:"maxValuePerCall"`
	RemainingValue  *hexutil.Big   `json:"remainingValue"`
	ValidAfter      hexutil.Uint64 `json:"validAfter"`
	ValidUntil      hexutil.Uint64 `json:"validUntil"`
	Active          bool           `json:"active"`
}

func (api *SmartAccountAPI) Status() SmartAccountStatus {
	return SmartAccountStatus{Version: smartAccountVersion, Consensus: false, RequiresFork: false, Deployed: false,
		AuthModes: map[string]uint8{"sessionKey": 1, "ownerSignatures": 2}}
}

func (api *SmartAccountAPI) OperationHash(op SmartUserOperation, entryPoint common.Address, chainID *hexutil.Big) (common.Hash, error) {
	if err := validateSmartOperation(op, entryPoint, chainID); err != nil {
		return common.Hash{}, err
	}
	args, err := smartABIArgs("bytes32", "address", "address", "uint256", "bytes32", "uint256", "uint48", "uint256", "address", "bytes32", "address", "uint256")
	if err != nil {
		return common.Hash{}, err
	}
	encoded, err := args.Pack(smartAccountTypeHash, op.Account, op.Target, smartBig(op.Value), crypto.Keccak256Hash(op.Data), smartBig(op.Nonce), new(big.Int).SetUint64(uint64(op.ValidUntil)), smartBig(op.GasLimit), op.Paymaster, crypto.Keccak256Hash(op.PaymasterData), entryPoint, smartBig(chainID))
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func (api *SmartAccountAPI) CreateData(req SmartAccountCreateRequest) (hexutil.Bytes, error) {
	if len(req.Owners) == 0 || len(req.Owners) > 32 {
		return nil, errors.New("owners must contain 1 to 32 addresses")
	}
	if req.Threshold == 0 || int(req.Threshold) > len(req.Owners) {
		return nil, errors.New("threshold exceeds owner count")
	}
	seen := make(map[common.Address]bool)
	for _, owner := range req.Owners {
		if owner == (common.Address{}) || seen[owner] {
			return nil, errors.New("owners must be non-zero and unique")
		}
		seen[owner] = true
	}
	return smartPack("createAccount(address[],uint16,bytes32)", req.Owners, req.Threshold, req.Salt)
}

func (api *SmartAccountAPI) ExecuteData(target common.Address, value *hexutil.Big, data hexutil.Bytes) (hexutil.Bytes, error) {
	if target == (common.Address{}) {
		return nil, errors.New("target is zero")
	}
	return smartPack("executeFromEntryPoint(address,uint256,bytes)", target, smartBig(value), []byte(data))
}

func (api *SmartAccountAPI) SetOwnersData(owners []common.Address, threshold uint16) (hexutil.Bytes, error) {
	_, err := api.CreateData(SmartAccountCreateRequest{Owners: owners, Threshold: threshold})
	if err != nil {
		return nil, err
	}
	return smartPack("setOwners(address[],uint16)", owners, threshold)
}

func (api *SmartAccountAPI) SetLimitsData(daily, highValue *hexutil.Big) (hexutil.Bytes, error) {
	return smartPack("setLimits(uint256,uint256)", smartBig(daily), smartBig(highValue))
}

func (api *SmartAccountAPI) SetGuardianData(guardian common.Address, enabled bool) (hexutil.Bytes, error) {
	if guardian == (common.Address{}) {
		return nil, errors.New("guardian is zero")
	}
	return smartPack("setGuardian(address,bool)", guardian, enabled)
}

func (api *SmartAccountAPI) SetRecoveryPolicyData(threshold uint16, delay hexutil.Uint64) (hexutil.Bytes, error) {
	if threshold == 0 {
		return nil, errors.New("guardian threshold is zero")
	}
	if uint64(delay) < 3600 || uint64(delay) > 30*86400 {
		return nil, errors.New("recovery delay must be between 1 hour and 30 days")
	}
	return smartPack("setRecoveryPolicy(uint16,uint48)", threshold, new(big.Int).SetUint64(uint64(delay)))
}

func (api *SmartAccountAPI) RecoveryHash(owners []common.Address, threshold uint16) (common.Hash, error) {
	if _, err := api.CreateData(SmartAccountCreateRequest{Owners: owners, Threshold: threshold}); err != nil {
		return common.Hash{}, err
	}
	args, _ := smartABIArgs("address[]", "uint16")
	encoded, err := args.Pack(owners, threshold)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func (api *SmartAccountAPI) ApproveRecoveryData(ownerHash common.Hash) (hexutil.Bytes, error) {
	return smartPack("approveRecovery(bytes32)", ownerHash)
}
func (api *SmartAccountAPI) CancelRecoveryData() (hexutil.Bytes, error) {
	return smartPack("cancelRecovery()")
}
func (api *SmartAccountAPI) CompleteRecoveryData(owners []common.Address, threshold uint16) (hexutil.Bytes, error) {
	if _, err := api.RecoveryHash(owners, threshold); err != nil {
		return nil, err
	}
	return smartPack("completeRecovery(address[],uint16)", owners, threshold)
}

func (api *SmartAccountAPI) SetSessionData(req SmartAccountSessionRequest) (hexutil.Bytes, error) {
	if req.Key == (common.Address{}) || req.Target == (common.Address{}) {
		return nil, errors.New("session key and target must be non-zero")
	}
	if len(req.Selector) != 4 {
		return nil, errors.New("selector must be exactly 4 bytes")
	}
	if req.ValidUntil <= req.ValidAfter {
		return nil, errors.New("session expiry must be after start")
	}
	tuple, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{{Name: "target", Type: "address"}, {Name: "selector", Type: "bytes4"}, {Name: "maxValuePerCall", Type: "uint128"}, {Name: "remainingValue", Type: "uint128"}, {Name: "validAfter", Type: "uint48"}, {Name: "validUntil", Type: "uint48"}, {Name: "active", Type: "bool"}})
	if err != nil {
		return nil, err
	}
	max, remaining := smartBig(req.MaxValuePerCall), smartBig(req.RemainingValue)
	if max.BitLen() > 128 || remaining.BitLen() > 128 {
		return nil, errors.New("session value exceeds uint128")
	}
	selector := [4]byte{}
	copy(selector[:], req.Selector)
	value := struct {
		Target          common.Address
		Selector        [4]byte
		MaxValuePerCall *big.Int
		RemainingValue  *big.Int
		ValidAfter      *big.Int
		ValidUntil      *big.Int
		Active          bool
	}{req.Target, selector, max, remaining, new(big.Int).SetUint64(uint64(req.ValidAfter)), new(big.Int).SetUint64(uint64(req.ValidUntil)), req.Active}
	encoded, err := abi.Arguments{{Type: mustSmartType("address")}, {Type: tuple}}.Pack(req.Key, value)
	if err != nil {
		return nil, err
	}
	return append(crypto.Keccak256([]byte("setSession(address,(address,bytes4,uint128,uint128,uint48,uint48,bool))"))[:4], encoded...), nil
}

func (api *SmartAccountAPI) RevokeSessionData(key common.Address) (hexutil.Bytes, error) {
	if key == (common.Address{}) {
		return nil, errors.New("session key is zero")
	}
	return smartPack("revokeSession(address)", key)
}

func (api *SmartAccountAPI) PredictAddress(factory common.Address, salt, initCodeHash common.Hash) (common.Address, error) {
	if factory == (common.Address{}) || initCodeHash == (common.Hash{}) {
		return common.Address{}, errors.New("factory and initCodeHash are required")
	}
	return common.BytesToAddress(crypto.Keccak256([]byte{0xff}, factory.Bytes(), salt.Bytes(), initCodeHash.Bytes())[12:]), nil
}

func validateSmartOperation(op SmartUserOperation, entryPoint common.Address, chainID *hexutil.Big) error {
	if op.Account == (common.Address{}) || op.Target == (common.Address{}) || entryPoint == (common.Address{}) {
		return errors.New("account, target, and entryPoint are required")
	}
	if chainID == nil || smartBig(chainID).Sign() <= 0 {
		return errors.New("chainId must be positive")
	}
	if op.Value == nil || op.Nonce == nil || op.GasLimit == nil {
		return errors.New("value, nonce, and gasLimit are required")
	}
	if smartBig(op.Value).Sign() < 0 || smartBig(op.Nonce).Sign() < 0 || smartBig(op.GasLimit).Sign() <= 0 {
		return errors.New("invalid numeric operation field")
	}
	if op.Paymaster == (common.Address{}) && len(op.PaymasterData) != 0 {
		return errors.New("paymasterData requires a paymaster")
	}
	return nil
}

func smartBig(v interface{}) *big.Int {
	switch n := v.(type) {
	case *hexutil.Big:
		if n != nil {
			return new(big.Int).Set((*big.Int)(n))
		}
	}
	return new(big.Int)
}

func smartPack(signature string, values ...interface{}) (hexutil.Bytes, error) {
	open := -1
	for i, c := range signature {
		if c == '(' {
			open = i
			break
		}
	}
	if open < 0 {
		return nil, errors.New("invalid signature")
	}
	types := signature[open+1 : len(signature)-1]
	var names []string
	if types != "" {
		names = splitSmartTypes(types)
	}
	args, err := smartABIArgs(names...)
	if err != nil {
		return nil, err
	}
	encoded, err := args.Pack(values...)
	if err != nil {
		return nil, err
	}
	return append(crypto.Keccak256([]byte(signature))[:4], encoded...), nil
}

func splitSmartTypes(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
		}
		if c == ',' && depth == 0 {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func smartABIArgs(types ...string) (abi.Arguments, error) {
	args := make(abi.Arguments, len(types))
	for i, name := range types {
		typ, err := abi.NewType(name, "", nil)
		if err != nil {
			return nil, err
		}
		args[i] = abi.Argument{Type: typ}
	}
	return args, nil
}
func mustSmartType(name string) abi.Type {
	typ, err := abi.NewType(name, "", nil)
	if err != nil {
		panic(err)
	}
	return typ
}
