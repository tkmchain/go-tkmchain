// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

// Package tkmasset defines the TKM-native asset identity carried by EVM
// contracts. It is deliberately separate from ERC interfaces: an ERC-20,
// ERC-721, or ERC-1155 implementation becomes a TKM asset only when it also
// carries this authenticated, chain-bound manifest.
package tkmasset

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// Magic is the unique TKM runtime-code trailer marker.
	Magic = "TKMASSET"
	// Version is the manifest encoding version.
	Version byte = 1

	maxNameBytes     = 96
	maxSymbolBytes   = 32
	maxMetadataBytes = 512

	precompileInputLength = 32 + common.AddressLength + 1 + common.HashLength
)

// Kind describes the accounting model of a TKM asset.
type Kind byte

const (
	KindFungible    Kind = 1 // TKM-20: one divisible balance per account.
	KindNonFungible Kind = 2 // TKM-721: one owner per token id.
	KindMulti       Kind = 3 // TKM-6909: many ids with fungible balances.
)

// Flags describe capabilities that wallets and explorers must surface before
// interacting with a contract. They are declarations, not a substitute for
// checking the contract's code and policy.
type Flags uint32

const (
	FlagMintable Flags = 1 << iota
	FlagBurnable
	FlagPausable
	FlagPermit
	FlagBatchTransfer
	FlagShielded
	FlagRoyalty
	FlagSoulbound
	FlagUpgradeable
)

const knownFlags = FlagMintable | FlagBurnable | FlagPausable | FlagPermit | FlagBatchTransfer | FlagShielded | FlagRoyalty | FlagSoulbound | FlagUpgradeable

var (
	ErrInvalidManifest       = errors.New("invalid TKM asset manifest")
	ErrManifestNotFound      = errors.New("TKM asset manifest not found in runtime code")
	ErrManifestChainMismatch = errors.New("TKM asset manifest chain ID does not match this chain")
)

// Manifest is the canonical, immutable identity published by a token
// contract. The deployed contract address is intentionally excluded from the
// bytes so the same source can be compiled reproducibly; AssetID binds it at
// the point of deployment.
type Manifest struct {
	Kind        Kind
	ChainID     *big.Int
	Decimals    uint8
	Flags       Flags
	PolicyHash  common.Hash
	Name        string
	Symbol      string
	MetadataURI string
}

func (k Kind) Valid() bool { return k >= KindFungible && k <= KindMulti }

func (k Kind) String() string {
	switch k {
	case KindFungible:
		return "fungible"
	case KindNonFungible:
		return "non-fungible"
	case KindMulti:
		return "multi"
	default:
		return "unknown"
	}
}

// Standard is the TKM-native standard name surfaced by RPC clients.
func (k Kind) Standard() string {
	switch k {
	case KindFungible:
		return "TKM-20"
	case KindNonFungible:
		return "TKM-721"
	case KindMulti:
		return "TKM-6909"
	default:
		return "TKM-unknown"
	}
}

func (m Manifest) Validate() error {
	if !m.Kind.Valid() || m.ChainID == nil || m.ChainID.Sign() < 0 || m.ChainID.BitLen() > 256 {
		return fmt.Errorf("%w: kind or chain ID", ErrInvalidManifest)
	}
	if m.Kind == KindNonFungible && m.Decimals != 0 {
		return fmt.Errorf("%w: NFTs must use zero decimals", ErrInvalidManifest)
	}
	if m.Flags & ^knownFlags != 0 {
		return fmt.Errorf("%w: unknown capability flags 0x%x", ErrInvalidManifest, uint32(m.Flags&^knownFlags))
	}
	if m.Flags&FlagSoulbound != 0 && m.Kind == KindFungible {
		return fmt.Errorf("%w: fungible assets cannot be soulbound", ErrInvalidManifest)
	}
	if !validText(m.Name, 1, maxNameBytes) || !validText(m.Symbol, 1, maxSymbolBytes) || !validText(m.MetadataURI, 0, maxMetadataBytes) {
		return fmt.Errorf("%w: invalid name, symbol, or metadata URI", ErrInvalidManifest)
	}
	return nil
}

func validText(value string, min, max int) bool {
	length := len([]byte(value))
	return length >= min && length <= max && utf8.ValidString(value)
}

// MarshalBinary encodes the canonical manifest. Its output is also the input
// to ManifestHash, so changes to any identity or policy field change the asset
// identity exposed by wallets and explorers.
func (m Manifest) MarshalBinary() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	chain := make([]byte, 32)
	m.ChainID.FillBytes(chain)
	name, symbol, metadata := []byte(m.Name), []byte(m.Symbol), []byte(m.MetadataURI)
	if len(name) > 0xffff || len(symbol) > 0xffff || len(metadata) > 0xffff {
		return nil, fmt.Errorf("%w: text field too long", ErrInvalidManifest)
	}
	result := make([]byte, 0, len(Magic)+1+1+32+1+4+32+6+len(name)+len(symbol)+len(metadata))
	result = append(result, []byte(Magic)...)
	result = append(result, Version, byte(m.Kind))
	result = append(result, chain...)
	result = append(result, m.Decimals)
	var flags [4]byte
	binary.BigEndian.PutUint32(flags[:], uint32(m.Flags))
	result = append(result, flags[:]...)
	result = append(result, m.PolicyHash.Bytes()...)
	for _, field := range [][]byte{name, symbol, metadata} {
		var length [2]byte
		binary.BigEndian.PutUint16(length[:], uint16(len(field)))
		result = append(result, length[:]...)
		result = append(result, field...)
	}
	return result, nil
}

func UnmarshalBinary(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) < len(Magic)+1+1+32+1+4+32+6 || !bytes.Equal(data[:len(Magic)], []byte(Magic)) || data[len(Magic)] != Version {
		return m, fmt.Errorf("%w: header", ErrInvalidManifest)
	}
	offset := len(Magic) + 1
	m.Kind = Kind(data[offset])
	offset++
	m.ChainID = new(big.Int).SetBytes(data[offset : offset+32])
	offset += 32
	m.Decimals = data[offset]
	offset++
	m.Flags = Flags(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	copy(m.PolicyHash[:], data[offset:offset+common.HashLength])
	offset += common.HashLength
	fields := []*string{&m.Name, &m.Symbol, &m.MetadataURI}
	for _, field := range fields {
		if offset+2 > len(data) {
			return Manifest{}, fmt.Errorf("%w: truncated text length", ErrInvalidManifest)
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+length > len(data) {
			return Manifest{}, fmt.Errorf("%w: truncated text field", ErrInvalidManifest)
		}
		*field = string(data[offset : offset+length])
		offset += length
	}
	if offset != len(data) {
		return Manifest{}, fmt.Errorf("%w: trailing bytes", ErrInvalidManifest)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// ManifestHash returns the commitment used in AssetID and explorer indexes.
func (m Manifest) ManifestHash() (common.Hash, error) {
	encoded, err := m.MarshalBinary()
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// AssetID returns the globally unique TKM identity for a deployed asset.
// Chain ID, contract address, kind, and manifest hash are all domain-separated
// from ordinary Ethereum token identifiers.
func AssetID(chainID *big.Int, contract common.Address, kind Kind, manifestHash common.Hash) (common.Hash, error) {
	if chainID == nil || chainID.Sign() < 0 || chainID.BitLen() > 256 || contract == (common.Address{}) || !kind.Valid() {
		return common.Hash{}, fmt.Errorf("%w: invalid asset ID inputs", ErrInvalidManifest)
	}
	chain := make([]byte, 32)
	chainID.FillBytes(chain)
	input := make([]byte, 0, len("TKM_ASSET_ID_V1")+32+common.AddressLength+1+common.HashLength)
	input = append(input, []byte("TKM_ASSET_ID_V1")...)
	input = append(input, chain...)
	input = append(input, contract.Bytes()...)
	input = append(input, byte(kind))
	input = append(input, manifestHash.Bytes()...)
	return crypto.Keccak256Hash(input), nil
}

// PrecompileInput builds the fixed-size argument for the TKM asset-ID
// precompile at 0x...f3.
func PrecompileInput(chainID *big.Int, contract common.Address, kind Kind, manifestHash common.Hash) ([]byte, error) {
	if chainID == nil || chainID.Sign() < 0 || chainID.BitLen() > 256 || contract == (common.Address{}) || !kind.Valid() {
		return nil, fmt.Errorf("%w: invalid precompile inputs", ErrInvalidManifest)
	}
	input := make([]byte, precompileInputLength)
	chainID.FillBytes(input[:32])
	copy(input[32:32+common.AddressLength], contract.Bytes())
	input[32+common.AddressLength] = byte(kind)
	copy(input[32+common.AddressLength+1:], manifestHash.Bytes())
	return input, nil
}

// AssetIDFromPrecompileInput validates and evaluates the fixed-size precompile
// input. It is exported so the VM and API share exactly one identity formula.
func AssetIDFromPrecompileInput(input []byte) (common.Hash, error) {
	if len(input) != precompileInputLength {
		return common.Hash{}, fmt.Errorf("%w: asset ID input must be %d bytes", ErrInvalidManifest, precompileInputLength)
	}
	chainID := new(big.Int).SetBytes(input[:32])
	contract := common.BytesToAddress(input[32 : 32+common.AddressLength])
	kind := Kind(input[32+common.AddressLength])
	manifestHash := common.BytesToHash(input[32+common.AddressLength+1:])
	return AssetID(chainID, contract, kind, manifestHash)
}

// AppendTrailer appends a self-delimiting manifest to runtime bytecode.
// Deployers should append this result to the runtime code returned by their
// compiler; the EVM executes the code prefix and tooling reads the trailer.
func AppendTrailer(runtime []byte, m Manifest) ([]byte, error) {
	canonical, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}
	// The trailer has its own marker and version, so omit the duplicate marker
	// and version from the payload while preserving the canonical encoding for
	// hashing and validation during parsing.
	payload := canonical[len(Magic)+1:]
	result := make([]byte, 0, len(runtime)+len(Magic)+1+4+len(payload))
	result = append(result, runtime...)
	result = append(result, []byte(Magic)...)
	result = append(result, Version)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	result = append(result, length[:]...)
	return append(result, payload...), nil
}

// ParseRuntimeCode extracts the final TKM manifest trailer. A normal Ethereum
// contract without the trailer is valid but is reported as non-native by RPC.
func ParseRuntimeCode(code []byte) (Manifest, bool, error) {
	marker := []byte(Magic)
	index := bytes.LastIndex(code, marker)
	if index < 0 {
		return Manifest{}, false, nil
	}
	if index+len(marker)+1+4 > len(code) || code[index+len(marker)] != Version {
		return Manifest{}, true, fmt.Errorf("%w: trailer header", ErrInvalidManifest)
	}
	lengthOffset := index + len(marker) + 1
	length := int(binary.BigEndian.Uint32(code[lengthOffset : lengthOffset+4]))
	payloadOffset := lengthOffset + 4
	if length == 0 || payloadOffset+length != len(code) {
		return Manifest{}, true, fmt.Errorf("%w: trailer length", ErrInvalidManifest)
	}
	canonical := make([]byte, 0, len(Magic)+1+length)
	canonical = append(canonical, []byte(Magic)...)
	canonical = append(canonical, Version)
	canonical = append(canonical, code[payloadOffset:]...)
	m, err := UnmarshalBinary(canonical)
	if err != nil {
		return Manifest{}, true, err
	}
	return m, true, nil
}
