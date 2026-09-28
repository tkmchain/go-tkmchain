// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

// TKMAssetAPI classifies EVM contract assets using the immutable TKM manifest
// trailer. Plain ERC contracts remain usable, but are reported as
// ethereum-compatible rather than TKM-native until they publish one.
type TKMAssetAPI struct {
	b tkmAssetStateBackend
}

type tkmAssetStateBackend interface {
	StateAndHeaderByNumberOrHash(context.Context, rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error)
}

type tkmAssetChainConfigBackend interface {
	ChainConfig() *params.ChainConfig
}

// TKMAssetInfo is the stable wallet/explorer representation of a contract's
// token identity.
type TKMAssetInfo struct {
	Address            common.Address `json:"address"`
	Native             bool           `json:"native"`
	Network            string         `json:"network"`
	Standard           string         `json:"standard"`
	Kind               string         `json:"kind"`
	ChainID            *hexutil.Big   `json:"chainId,omitempty"`
	Name               string         `json:"name,omitempty"`
	Symbol             string         `json:"symbol,omitempty"`
	Decimals           uint8          `json:"decimals,omitempty"`
	Flags              uint32         `json:"flags,omitempty"`
	PolicyHash         common.Hash    `json:"policyHash,omitempty"`
	MetadataURI        string         `json:"metadataURI,omitempty"`
	ManifestHash       common.Hash    `json:"manifestHash,omitempty"`
	AssetID            common.Hash    `json:"assetId,omitempty"`
	RuntimeCodeHash    common.Hash    `json:"runtimeCodeHash"`
	Manifest           hexutil.Bytes  `json:"manifest,omitempty"`
	ClassificationNote string         `json:"classificationNote,omitempty"`
}

// TKMAssetManifestRequest is accepted by tkmasset_buildManifest.
type TKMAssetManifestRequest struct {
	ChainID     *hexutil.Big    `json:"chainId"`
	Contract    *common.Address `json:"contract,omitempty"`
	Kind        string          `json:"kind"`
	Decimals    uint8           `json:"decimals"`
	Flags       uint32          `json:"flags"`
	PolicyHash  common.Hash     `json:"policyHash"`
	Name        string          `json:"name"`
	Symbol      string          `json:"symbol"`
	MetadataURI string          `json:"metadataURI"`
}

type TKMAssetManifestResult struct {
	Standard     string        `json:"standard"`
	Manifest     hexutil.Bytes `json:"manifest"`
	Trailer      hexutil.Bytes `json:"runtimeTrailer"`
	ManifestHash common.Hash   `json:"manifestHash"`
	AssetID      common.Hash   `json:"assetId,omitempty"`
}

var errTKMAssetBackendUnavailable = errors.New("TKM asset state backend is not available")

func NewTKMAssetAPI(b tkmAssetStateBackend) *TKMAssetAPI { return &TKMAssetAPI{b: b} }

func tkmAssetBlockNumberOrHash(blockNrOrHash *rpc.BlockNumberOrHash) rpc.BlockNumberOrHash {
	if blockNrOrHash == nil {
		return rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	}
	return *blockNrOrHash
}

// BuildManifest returns the canonical bytes and runtime trailer to append to
// a token's deployed runtime bytecode.
func (api *TKMAssetAPI) BuildManifest(req TKMAssetManifestRequest) (*TKMAssetManifestResult, error) {
	if req.ChainID == nil {
		return nil, fmt.Errorf("%w: chainId is required", tkmasset.ErrInvalidManifest)
	}
	kind, err := parseTKMAssetKind(req.Kind)
	if err != nil {
		return nil, err
	}
	m := tkmasset.Manifest{Kind: kind, ChainID: (*big.Int)(req.ChainID), Decimals: req.Decimals, Flags: tkmasset.Flags(req.Flags), PolicyHash: req.PolicyHash, Name: req.Name, Symbol: req.Symbol, MetadataURI: req.MetadataURI}
	canonical, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}
	trailerCode, err := tkmasset.AppendTrailer(nil, m)
	if err != nil {
		return nil, err
	}
	hash, err := m.ManifestHash()
	if err != nil {
		return nil, err
	}
	result := &TKMAssetManifestResult{Standard: kind.Standard(), Manifest: canonical, Trailer: trailerCode, ManifestHash: hash}
	if req.Contract != nil {
		result.AssetID, err = tkmasset.AssetID(m.ChainID, *req.Contract, kind, hash)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetAsset returns a deterministic classification for a contract at a
// canonical state. Missing manifests are a normal result for Ethereum-style
// contracts and do not make the RPC call fail.
func (api *TKMAssetAPI) GetAsset(ctx context.Context, address common.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*TKMAssetInfo, error) {
	if api.b == nil {
		return nil, errTKMAssetBackendUnavailable
	}
	st, _, err := api.b.StateAndHeaderByNumberOrHash(ctx, tkmAssetBlockNumberOrHash(blockNrOrHash))
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, errTKMAssetBackendUnavailable
	}
	code := st.GetCode(address)
	if err := st.Error(); err != nil {
		return nil, err
	}
	info := &TKMAssetInfo{Address: address, Native: false, Network: "ethereum-compatible", Standard: "unknown", Kind: "unknown", RuntimeCodeHash: crypto.Keccak256Hash(code), ClassificationNote: "runtime code has no TKM asset manifest"}
	m, found, err := tkmasset.ParseRuntimeCode(code)
	if err != nil {
		return nil, err
	}
	if !found {
		return info, nil
	}
	hash, err := m.ManifestHash()
	if err != nil {
		return nil, err
	}
	info.Manifest = mustManifestBytes(m)
	info.ManifestHash = hash
	info.Network = "tkm"
	info.Standard = m.Kind.Standard()
	info.Kind = m.Kind.String()
	info.Name, info.Symbol, info.Decimals, info.Flags, info.PolicyHash, info.MetadataURI = m.Name, m.Symbol, m.Decimals, uint32(m.Flags), m.PolicyHash, m.MetadataURI
	info.ChainID = (*hexutil.Big)(new(big.Int).Set(m.ChainID))
	if cfg, ok := api.b.(tkmAssetChainConfigBackend); ok && cfg.ChainConfig() != nil && cfg.ChainConfig().ChainID != nil && cfg.ChainConfig().ChainID.Cmp(m.ChainID) != 0 {
		info.ClassificationNote = tkmasset.ErrManifestChainMismatch.Error()
		return info, nil
	}
	info.Native = true
	info.ClassificationNote = "TKM-native manifest verified"
	info.AssetID, err = tkmasset.AssetID(m.ChainID, address, m.Kind, hash)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// Classify is an alias with a name that reads naturally in clients.
func (api *TKMAssetAPI) Classify(ctx context.Context, address common.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*TKMAssetInfo, error) {
	return api.GetAsset(ctx, address, blockNrOrHash)
}

func mustManifestBytes(m tkmasset.Manifest) hexutil.Bytes {
	encoded, _ := m.MarshalBinary()
	return encoded
}

func parseTKMAssetKind(value string) (tkmasset.Kind, error) {
	switch value {
	case "fungible", "TKM-20", "tkm-20":
		return tkmasset.KindFungible, nil
	case "non-fungible", "nft", "TKM-721", "tkm-721":
		return tkmasset.KindNonFungible, nil
	case "multi", "TKM-6909", "tkm-6909":
		return tkmasset.KindMulti, nil
	default:
		return 0, fmt.Errorf("%w: unknown kind %q", tkmasset.ErrInvalidManifest, value)
	}
}
