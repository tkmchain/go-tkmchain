package eth

import "github.com/ethereum/go-ethereum/common/hexutil"

// ShieldedV5Status advertises the separately scheduled 16-input relation.
// Shield3/4 remain active and byte-compatible before this timestamp.
type ShieldedV5Status struct {
	ShieldedV4Status
	Shield5Time *hexutil.Uint64 `json:"shield5Time"`
}

func (api *PrivacyAPI) ShieldedV5Status() ShieldedV5Status {
	status := ShieldedV5Status{ShieldedV4Status: api.ShieldedV4Status()}
	status.MaxInputs = 16
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return status
	}
	cfg := api.e.blockchain.Config()
	if cfg.Shield5Time != nil {
		t := hexutil.Uint64(*cfg.Shield5Time)
		status.Shield5Time = &t
	}
	head := api.e.blockchain.CurrentBlock()
	status.Active = head != nil && cfg.IsShield5(head.Number, head.Time)
	return status
}
