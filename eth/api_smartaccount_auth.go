package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// OwnerAuthorization encodes sorted ECDSA signatures for authorization mode 2.
// The contracts recover and verify the actual owner addresses; addresses supplied
// by callers are never trusted.
func (api *SmartAccountAPI) OwnerAuthorization(signatures []hexutil.Bytes) (hexutil.Bytes, error) {
	if len(signatures) == 0 || len(signatures) > 32 {
		return nil, errors.New("owner authorization requires 1 to 32 signatures")
	}
	values := make([][]byte, len(signatures))
	for i, signature := range signatures {
		if len(signature) != crypto.SignatureLength {
			return nil, errors.New("each owner signature must be 65 bytes")
		}
		values[i] = append([]byte(nil), signature...)
	}
	args, err := smartABIArgs("bytes[]")
	if err != nil {
		return nil, err
	}
	encoded, err := args.Pack(values)
	if err != nil {
		return nil, err
	}
	return append(hexutil.Bytes{2}, encoded...), nil
}

// SessionAuthorization prefixes one ECDSA signature with session-key mode 1.
func (api *SmartAccountAPI) SessionAuthorization(signature hexutil.Bytes) (hexutil.Bytes, error) {
	if len(signature) != crypto.SignatureLength {
		return nil, errors.New("session signature must be 65 bytes")
	}
	return append(hexutil.Bytes{1}, signature...), nil
}

// SponsorshipHash returns the exact digest signed by an allowlist paymaster.
func (api *SmartAccountAPI) SponsorshipHash(operationHash common.Hash, expiry hexutil.Uint64, paymaster common.Address, chainID *hexutil.Big) (common.Hash, error) {
	if operationHash == (common.Hash{}) || paymaster == (common.Address{}) {
		return common.Hash{}, errors.New("operationHash and paymaster are required")
	}
	if expiry == 0 || chainID == nil || smartBig(chainID).Sign() <= 0 {
		return common.Hash{}, errors.New("expiry and positive chainId are required")
	}
	args, err := smartABIArgs("string", "bytes32", "uint48", "address", "uint256")
	if err != nil {
		return common.Hash{}, err
	}
	encoded, err := args.Pack("TKM_SPONSOR_V1", operationHash, new(big.Int).SetUint64(uint64(expiry)), paymaster, smartBig(chainID))
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// SponsorshipData encodes the expiry and ECDSA signature accepted by the paymaster.
func (api *SmartAccountAPI) SponsorshipData(expiry hexutil.Uint64, signature hexutil.Bytes) (hexutil.Bytes, error) {
	if expiry == 0 || len(signature) != crypto.SignatureLength {
		return nil, errors.New("expiry and a 65-byte signature are required")
	}
	args, err := smartABIArgs("uint48", "bytes")
	if err != nil {
		return nil, err
	}
	return args.Pack(new(big.Int).SetUint64(uint64(expiry)), []byte(signature))
}
