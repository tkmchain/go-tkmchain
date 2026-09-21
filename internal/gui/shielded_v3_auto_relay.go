package gui

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/internal/shield3wallet"
)

func (g *GUI) shield3AutoRelay(ctx context.Context, record *shield3RequestRecord, requestID string) (map[string]any, error) {
	if record.RelayURL == "" {
		return nil, errors.New("this draft has no automatic relay; use its exported packet")
	}
	status, err := g.shield3RelayStatus(ctx, record)
	if err != nil {
		return nil, err
	}
	if status["status"] == "confirmed" || status["status"] == "unconfirmed" {
		return status, nil
	}
	if status["status"] != "prepared · notes reserved" {
		return nil, errors.New("relay draft expired or its inputs conflict; check canonical status")
	}
	response, err := shield3wallet.SubmitRelayPacketWithConfig(ctx, record.RelayURL, requestID, record.Raw, g.opts.RelayTransport)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "unconfirmed", "transactionHash": response.TransactionHash, "submissionUncertain": response.SubmissionUncertain}, nil
}
