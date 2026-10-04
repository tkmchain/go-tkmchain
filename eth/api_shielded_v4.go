package eth

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/zk/shielded4"
)

// ShieldedV4Status reports the native relation and the single Antartical
// activation gate. Shield4 has no independent fork timestamp.
type ShieldedV4Status struct {
	ConfidentialLedger     bool            `json:"confidentialLedger"`
	ConfidentialLedgerTime *hexutil.Uint64 `json:"confidentialLedgerTime"`
	PrepaidFeeGas          uint64          `json:"prepaidFeeGas"`
	Active                 bool            `json:"active"`
	NativeVerifier         bool            `json:"nativeVerifier"`
	ActivationTime         *hexutil.Uint64 `json:"activationTime"`
	MaxSendWei             string          `json:"maxSendWei"`
	MaxRecipients          uint64          `json:"maxRecipients"`
	ViewKeyVersion         uint64          `json:"viewKeyVersion"`
	MaxInputs              uint64          `json:"maxInputs"`
	MaxSendTKM             uint64          `json:"maxSendTKM"`
	ShieldedOnly           bool            `json:"shieldedOnly"`
	ShieldedOnlyTime       *hexutil.Uint64 `json:"shieldedOnlyTime"`
}

func (api *PrivacyAPI) ShieldedV4Status() ShieldedV4Status {
	status := ShieldedV4Status{MaxRecipients: shielded4.OutputSlots - 1, ViewKeyVersion: 2, MaxInputs: shielded4.InputSlots, NativeVerifier: shielded4.NativeAvailable(), MaxSendWei: shielded4.MaxSendWei().String(), MaxSendTKM: shielded4.MaxSendTKM}
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return status
	}
	cfg := api.e.blockchain.Config()
	status.PrepaidFeeGas = core.ConfidentialPrepaidGas
	if cfg.ConfidentialLedgerTime != nil {
		t := hexutil.Uint64(*cfg.ConfidentialLedgerTime)
		status.ConfidentialLedgerTime = &t
	}
	if cfg.ShieldedOnlyTime != nil {
		t := hexutil.Uint64(*cfg.ShieldedOnlyTime)
		status.ShieldedOnlyTime = &t
	}
	if cfg.AntarticalTime != nil {
		t := hexutil.Uint64(*cfg.AntarticalTime)
		status.ActivationTime = &t
	}
	head := api.e.blockchain.CurrentBlock()
	status.Active = head != nil && cfg.IsAntartical(head.Number, head.Time) && cfg.IsPrivacyCommitments(head.Number, head.Time)
	status.ShieldedOnly = head != nil && cfg.IsShieldedOnly(head.Number, head.Time)
	status.ConfidentialLedger = head != nil && cfg.IsConfidentialLedger(head.Number, head.Time)
	return status
}

func (api *PrivacyAPI) ShieldedV4Path(commitment shielded4.Digest) (core.ShieldedV3Path, error) {
	return api.ShieldedV3Path(commitment)
}
