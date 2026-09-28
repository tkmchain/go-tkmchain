package randomx

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// verifyBlockHashAnchor enforces the Antartical header commitment on every
// RandomX validation path, including header-only sync. parentCandidates is
// used by VerifyHeaders for parents in the same batch that are not in the DB.
func verifyBlockHashAnchor(chain consensus.ChainHeaderReader, header *types.Header, parentCandidates []*types.Header) error {
	if header == nil || header.Number == nil || chain == nil || chain.Config() == nil {
		return nil
	}
	config := chain.Config()
	// The anchor envelope is a TKM RandomX consensus rule. Other engines in
	// go-ethereum (notably Clique test chains) retain their historical header
	// formats and must not be interpreted as TKM blocks.
	if config.RandomX == nil {
		return nil
	}
	anchor, found, err := antartical.BlockHashAnchorFromHeaderExtra(header.Extra)
	if err != nil {
		return err
	}
	active := config.IsAntartical(header.Number, header.Time)
	if !active {
		if found || uint64(len(header.Extra)) > params.MaximumExtraDataSize {
			return fmt.Errorf("invalid pre-Antartical header extra data")
		}
		return nil
	}
	if !found {
		return antartical.ErrMissingBlockHashAnchor
	}
	if uint64(len(header.Extra)) > params.AntarticalMaximumExtraDataSize {
		return fmt.Errorf("Antartical header extra data exceeds %d bytes", params.AntarticalMaximumExtraDataSize)
	}
	if header.Number.Sign() == 0 {
		return antartical.ErrInvalidBlockHashAnchor
	}
	var parent *types.Header
	for i := len(parentCandidates) - 1; i >= 0; i-- {
		candidate := parentCandidates[i]
		if candidate != nil && candidate.Hash() == header.ParentHash && candidate.Number != nil && candidate.Number.Uint64()+1 == header.Number.Uint64() {
			parent = candidate
			break
		}
	}
	if parent == nil {
		parent = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	}
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	previousRolling := common.Hash{}
	parentAnchor, parentHasAnchor, err := antartical.BlockHashAnchorFromHeaderExtra(parent.Extra)
	if err != nil {
		return err
	}
	if config.IsAntartical(parent.Number, parent.Time) {
		// Genesis has no predecessor and therefore cannot carry an anchor even
		// on Egypt, where Antartical is active from genesis.
		if parent.Number.Sign() > 0 && !parentHasAnchor {
			return antartical.ErrMissingBlockHashAnchor
		}
		if parentHasAnchor {
			previousRolling = parentAnchor.Rolling
		}
	} else if parentHasAnchor {
		return fmt.Errorf("pre-Antartical parent contains block-hash anchor")
	}
	return antartical.ValidateBlockHashAnchor(anchor, header.Number.Uint64(), header.ParentHash, previousRolling)
}
