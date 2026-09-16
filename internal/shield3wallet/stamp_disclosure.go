package shield3wallet

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

// StampDisclosure opens one self-declared name/country record, not the master
// stamp KEM seed, which can also recognize encrypted output stamp records.
type StampDisclosure struct {
	Version   uint64                         `json:"version"`
	Scope     string                         `json:"scope"`
	Record    pqcrypto.ShieldedV3StampRecord `json:"record"`
	PublicKey hexutil.Bytes                  `json:"publicKey"`
	RecordKey hexutil.Bytes                  `json:"recordKey"`
}
type StampDisclosureResult struct {
	Address common.Address `json:"address"`
	ChainID uint64         `json:"chainId"`
	Name    string         `json:"name"`
	Country string         `json:"country"`
}

func ExportStampDisclosure(seed []byte, identity *Identity) (StampDisclosure, error) {
	if identity == nil || identity.Stamp == nil {
		return StampDisclosure{}, errors.New("stamped identity required")
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return StampDisclosure{}, err
	}
	pub := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil || address != identity.Address || !pqcrypto.VerifyShieldedV3Stamp(pub, identity.Stamp) {
		return StampDisclosure{}, errors.New("stamp does not match this account")
	}
	context := pqcrypto.ShieldedV3Context{ChainID: identity.ChainID, Purpose: pqcrypto.ShieldedV3Stamp, Commitment: identity.Stamp.Commitment}
	recordKey, err := pqcrypto.ShieldedV3RecordKey(identity.StampSeed, identity.Stamp.Ciphertext, context)
	if err != nil {
		return StampDisclosure{}, err
	}
	return StampDisclosure{Version: 2, Scope: "stamp", Record: *identity.Stamp, PublicKey: pub, RecordKey: recordKey}, nil
}
func VerifyStampDisclosure(disclosure StampDisclosure) (StampDisclosureResult, error) {
	if disclosure.Version != 2 || disclosure.Scope != "stamp" || !pqcrypto.VerifyShieldedV3Stamp(disclosure.PublicKey, &disclosure.Record) {
		return StampDisclosureResult{}, errors.New("invalid or unauthenticated stamp disclosure")
	}
	context := pqcrypto.ShieldedV3Context{ChainID: disclosure.Record.ChainID, Purpose: pqcrypto.ShieldedV3Stamp, Commitment: disclosure.Record.Commitment}
	plain, err := pqcrypto.OpenShieldedV3RecordKey(disclosure.RecordKey, disclosure.Record.Ciphertext, context)
	if err != nil {
		return StampDisclosureResult{}, err
	}
	defer clear(plain)
	var text pqcrypto.ShieldedV3StampText
	if err := json.Unmarshal(plain, &text); err != nil || text.Name == "" || text.Country == "" || len(text.Name) > 120 || len(text.Country) > 80 || !utf8.ValidString(text.Name) || !utf8.ValidString(text.Country) {
		return StampDisclosureResult{}, errors.New("invalid stamp labels")
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, disclosure.PublicKey)
	if err != nil {
		return StampDisclosureResult{}, err
	}
	return StampDisclosureResult{Address: address, ChainID: disclosure.Record.ChainID, Name: text.Name, Country: text.Country}, nil
}
