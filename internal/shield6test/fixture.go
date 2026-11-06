// Package shield6test carries public, ephemeral test data between the expensive
// canonical proof rehearsal and the TCP rehearsal. It is not a setup format or
// a source of production key provenance. No proving key or witness is included.
package shield6test

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

const (
	ChainID            = 18979
	FixtureEnvironment = "SHIELD6_RECIPIENT_NETWORK_FIXTURE"
	fixtureVersion     = "shield6-recipient-test-fixture-v2"
	maxFixtureSize     = 16 << 20
)

type Fixture struct {
	Version     string              `json:"version"`
	Config      *params.ChainConfig `json:"config"`
	Timestamp   uint64              `json:"timestamp"`
	Alloc       types.GenesisAlloc  `json:"alloc"`
	Payment     hexutil.Bytes       `json:"payment"`
	Attacks     []hexutil.Bytes     `json:"attacks"`
	Sender      common.Address      `json:"sender"`
	OutputCount uint64              `json:"outputCount"`
	SenderNonce uint64              `json:"senderNonce"`
}

// Write exclusively creates a fixture only after the canonical proof test
// passes. A stale file is never silently replaced or reused by that test.
func Write(path string, fixture Fixture) error {
	fixture.Version = fixtureVersion
	encoded, err := json.Marshal(fixture)
	if err != nil {
		return err
	}
	if len(encoded) > maxFixtureSize {
		return errors.New("Shield6 network test fixture exceeds size limit")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(path)
		return writeErr
	}
	return closeErr
}

func Read(path string) (*Fixture, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxFixtureSize {
		return nil, errors.New("invalid Shield6 network test fixture size")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxFixtureSize+1))
	decoder.DisallowUnknownFields()
	var fixture Fixture
	if err := decoder.Decode(&fixture); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || fixture.Version != fixtureVersion || fixture.Config == nil || fixture.Config.ChainID == nil || !fixture.Config.ChainID.IsUint64() || fixture.Config.ChainID.Uint64() != ChainID || len(fixture.Alloc) == 0 || len(fixture.Payment) == 0 || len(fixture.Attacks) != 4 {
		return nil, errors.New("invalid Shield6 network test fixture")
	}
	return &fixture, nil
}
