package eth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/tkmnet"
)

const tkmNameMaxBatch = shield3wallet.UsernameLookupBatchLen

// TkmNameAPI serves signed Shield3 username records and reconciles its
// persistent directory against the canonical chain before reads.
type TkmNameAPI struct {
	directory     *shield3wallet.UsernameDirectory
	eth           *Ethereum
	initErr       error
	networkMu     sync.RWMutex
	proxy         string
	port          string
	transit       []tkmnet.Descriptor
	directories   []tkmnet.PinnedDirectoryPeer
	publicationMu sync.Mutex
	networkReady  bool
	published     map[string][32]byte
}

// TkmNameRecord is the public, owner-signed binding returned in a lookup batch.
type TkmNameRecord = shield3wallet.UsernameBinding

type TkmNameDirectoryStatus struct {
	ChainID                uint64      `json:"chainId"`
	HeadNumber             uint64      `json:"headNumber"`
	HeadHash               common.Hash `json:"headHash"`
	DirectoryHead          uint64      `json:"directoryHead"`
	DirectoryHash          common.Hash `json:"directoryHash"`
	Records                int         `json:"records"`
	NetworkActive          bool        `json:"networkActive"`
	NetworkPeersConfigured bool        `json:"networkPeersConfigured"`
	DirectoryOperators     int         `json:"directoryOperators"`
	NetworkReady           bool        `json:"networkReady"`
}

func NewTkmNameAPI(e *Ethereum) *TkmNameAPI {
	if e == nil || e.chainDb == nil || e.BlockChain() == nil || e.BlockChain().Config() == nil || e.BlockChain().Config().ChainID == nil {
		return &TkmNameAPI{}
	}
	directory, err := shield3wallet.NewPersistentUsernameDirectory(e.BlockChain().Config().ChainID.Uint64(), e.chainDb)
	if err != nil {
		return &TkmNameAPI{initErr: err}
	}
	return &TkmNameAPI{directory: directory, eth: e}
}

// Register stores an owner-signed record and, once activated, publishes it to
// every configured pinned directory over independent TKMNet/Tor circuits.
func (api *TkmNameAPI) Register(record TkmNameRecord) (bool, error) {
	if err := api.ready(); err != nil {
		return false, err
	}
	if err := api.syncCanonical(); err != nil {
		return false, err
	}
	if len(record.PaymentCode) > shield3wallet.UsernamePaymentCodeMax || len(record.Signature) > shield3wallet.UsernameSignatureMax {
		return false, errors.New("username binding exceeds its size limits")
	}
	payload, err := shield3wallet.DecodePaymentCode(record.PaymentCode, record.ChainID)
	if err != nil {
		return false, err
	}
	state, err := api.eth.currentPrivacyState()
	if err != nil {
		return false, err
	}
	stamp, err := core.AntarticalStampForAddress(state, payload.Address)
	if err != nil {
		return false, err
	}
	if !stamp.Registered || stamp.Owner != payload.Owner || stamp.Commitment != payload.Stamp.Commitment {
		return false, errors.New("username registration requires a confirmed Shield3 address stamp")
	}
	head := api.eth.BlockChain().CurrentHeader()
	if head == nil || head.Number == nil {
		return false, errors.New("canonical chain head is unavailable")
	}
	if err := api.directory.PutAt(record, time.Now().UTC(), head.Number.Uint64(), head.Hash()); err != nil {
		return false, err
	}
	if api.usernameNetworkActive() {
		api.publicationMu.Lock()
		api.networkReady = false
		delete(api.published, record.Username)
		api.publicationMu.Unlock()
		if err := api.syncNetworkDirectory(); err != nil {
			return false, fmt.Errorf("username stored locally but directory replication is incomplete; retry registration: %w", err)
		}
	}
	return true, nil
}

// LookupBatch accepts exactly 16 distinct canonical usernames and returns all
// 16 records in the submitted order. Missing or expired entries fail the
// entire request, avoiding a response-side marker for the real target.
func (api *TkmNameAPI) LookupBatch(names []string) ([]TkmNameRecord, error) {
	if err := api.ready(); err != nil {
		return nil, err
	}
	if err := api.syncCanonical(); err != nil {
		return nil, err
	}
	if len(names) != tkmNameMaxBatch {
		return nil, errors.New("private username lookup requires exactly 16 entries")
	}
	return api.directory.LookupBatch(names, time.Now().UTC())
}

// ResolveLocal is intended for local wallet RPC only. It is deliberately not
// a substitute for LookupBatch on public or remotely reachable endpoints.
func (api *TkmNameAPI) ResolveLocal(handle string) (TkmNameRecord, error) {
	if err := api.ready(); err != nil {
		return TkmNameRecord{}, err
	}
	if err := api.syncCanonical(); err != nil {
		return TkmNameRecord{}, err
	}
	return api.directory.Resolve(handle, time.Now().UTC())
}

// Resolve routes activated lookups over TKMNet. Before the dedicated
// username-network fork it retains local-only behavior for compatibility.
func (api *TkmNameAPI) Resolve(handle string) (TkmNameRecord, error) {
	if err := api.ready(); err != nil {
		return TkmNameRecord{}, err
	}
	if err := api.syncCanonical(); err != nil {
		return TkmNameRecord{}, err
	}
	if !api.usernameNetworkActive() {
		return api.ResolveLocal(handle)
	}
	if err := api.syncNetworkDirectory(); err != nil {
		return TkmNameRecord{}, fmt.Errorf("username directory synchronization failed: %w", err)
	}
	chainID := api.eth.BlockChain().Config().ChainID.Uint64()
	name, err := shield3wallet.ParseUsernameHandle(handle, chainID)
	if err != nil {
		return TkmNameRecord{}, err
	}
	proxy, port, transit, directories := api.networkConfig()
	if proxy == "" || port == "" || len(transit) < 2 || len(directories) < 2 {
		return TkmNameRecord{}, errors.New("username network is not ready: configure Tor, two independent transit relays, and at least two pinned directory operators")
	}
	if err := validateUsernameNetworkConfig(proxy, port, transit, directories, time.Now()); err != nil {
		return TkmNameRecord{}, err
	}
	coverPeer, err := chooseUsernamePeer(directories)
	if err != nil {
		return TkmNameRecord{}, err
	}
	coverRoute, err := chooseUsernameRoute(transit, coverPeer.Descriptor)
	if err != nil {
		return TkmNameRecord{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	coverReply, err := tkmnet.ExchangeService(ctx, proxy, port, coverRoute, tkmnet.ServiceUsername, tkmnet.UsernameCoverRequest())
	if err != nil {
		return TkmNameRecord{}, fmt.Errorf("fetch private username cover set: %w", err)
	}
	covers, err := tkmnet.DecodeUsernameCoverReply(coverReply)
	if err != nil {
		return TkmNameRecord{}, err
	}
	names := make([]string, 0, tkmnet.UsernameLookupFanout)
	names = append(names, name)
	seen := map[string]bool{name: true}
	for _, candidate := range covers {
		if len(names) >= tkmnet.UsernameLookupFanout || len(names) >= len(directories) {
			break
		}
		if !seen[candidate] {
			names = append(names, candidate)
			seen[candidate] = true
		}
	}
	// Include the cover source among query operators. This permits a useful
	// one-directory deployment (one direct query) and two-directory deployment
	// (target plus one cover) while keeping each alias on its own circuit.
	queries, err := tkmnet.PlanPrivateUsernameLookup(name, names, directories, uint64(time.Now().Unix()), rand.Reader)
	if err != nil {
		return TkmNameRecord{}, err
	}
	type lookupResult struct {
		record TkmNameRecord
		err    error
	}
	results := make(chan lookupResult, len(queries))
	for _, query := range queries {
		query := query
		go func() {
			route, err := chooseUsernameRoute(transit, query.Peer)
			if err != nil {
				results <- lookupResult{err: err}
				return
			}
			request, err := tkmnet.EncodeUsernameQuery(query.Name)
			if err != nil {
				results <- lookupResult{err: err}
				return
			}
			response, err := tkmnet.ExchangeService(ctx, proxy, port, route, tkmnet.ServiceUsername, request)
			if err != nil {
				results <- lookupResult{err: fmt.Errorf("private username lookup failed: %w", err)}
				return
			}
			encoded, err := tkmnet.DecodeUsernameRecordReply(response)
			if err != nil {
				results <- lookupResult{err: err}
				return
			}
			var record TkmNameRecord
			if err := rlp.DecodeBytes(encoded, &record); err != nil {
				results <- lookupResult{err: errors.New("directory returned malformed username binding")}
				return
			}
			if err := shield3wallet.VerifyUsernameBinding(record, chainID, time.Now().UTC()); err != nil {
				results <- lookupResult{err: fmt.Errorf("directory returned unauthenticated binding: %w", err)}
				return
			}
			if record.Username != query.Name {
				results <- lookupResult{err: errors.New("directory returned a binding for a different username")}
				return
			}
			results <- lookupResult{record: record}
		}()
	}
	var resolved TkmNameRecord
	for range queries {
		result := <-results
		if result.err != nil {
			cancel()
			return TkmNameRecord{}, result.err
		}
		if result.record.Username == name {
			resolved = result.record
		}
	}
	if resolved.Username == "" {
		return TkmNameRecord{}, errors.New("private username lookup did not return its target")
	}
	return resolved, nil
}

// Status reports how far the local alias DB has been reconciled against the
// canonical chain. NetworkReady remains false until TKMNet directory peers
// are configured and synchronized; local height alone is not global sync.
func (api *TkmNameAPI) Status() (TkmNameDirectoryStatus, error) {
	if err := api.ready(); err != nil {
		return TkmNameDirectoryStatus{}, err
	}
	if err := api.syncCanonical(); err != nil {
		return TkmNameDirectoryStatus{}, err
	}
	head := api.eth.BlockChain().CurrentHeader()
	chainID := api.eth.BlockChain().Config().ChainID.Uint64()
	directoryHead := api.directory.ChainHead()
	_, _, _, directories := api.networkConfig()
	return TkmNameDirectoryStatus{
		ChainID: chainID, HeadNumber: head.Number.Uint64(), HeadHash: head.Hash(),
		DirectoryHead: directoryHead.BlockNumber, DirectoryHash: directoryHead.BlockHash,
		Records: api.directory.Count(), NetworkActive: api.usernameNetworkActive(),
		NetworkPeersConfigured: api.networkConfigured(), DirectoryOperators: len(directories),
		NetworkReady: api.isNetworkReady(),
	}, nil
}

// ConfigureNetwork installs the local Tor egress path and pinned directory
// operator set. Descriptor slices are copied to protect live routing state.
func (api *TkmNameAPI) ConfigureNetwork(proxy, port string, transit []tkmnet.Descriptor, directories []tkmnet.PinnedDirectoryPeer) {
	if api == nil {
		return
	}
	api.networkMu.Lock()
	defer api.networkMu.Unlock()
	api.proxy, api.port = proxy, port
	api.transit = append([]tkmnet.Descriptor(nil), transit...)
	api.directories = append([]tkmnet.PinnedDirectoryPeer(nil), directories...)
}

func (api *TkmNameAPI) networkConfig() (string, string, []tkmnet.Descriptor, []tkmnet.PinnedDirectoryPeer) {
	api.networkMu.RLock()
	defer api.networkMu.RUnlock()
	return api.proxy, api.port, append([]tkmnet.Descriptor(nil), api.transit...), append([]tkmnet.PinnedDirectoryPeer(nil), api.directories...)
}

func (api *TkmNameAPI) networkConfigured() bool {
	proxy, port, transit, peers := api.networkConfig()
	if proxy == "" || port == "" || len(transit) < 2 || len(peers) < 2 {
		return false
	}
	return validateUsernameNetworkConfig(proxy, port, transit, peers, time.Now()) == nil
}

func validateUsernameNetworkConfig(proxy, port string, transit []tkmnet.Descriptor, directories []tkmnet.PinnedDirectoryPeer, now time.Time) error {
	if _, err := tkmnet.NewSOCKS5Dialer(proxy); err != nil {
		return fmt.Errorf("invalid TKMNet Tor proxy: %w", err)
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNum == 0 {
		return errors.New("invalid TKMNet onion relay port")
	}
	if err := validatePinnedDirectorySet(directories, now); err != nil {
		return err
	}
	directoryIDs := make(map[[32]byte]bool, len(directories))
	directoryOnions := make(map[string]bool, len(directories))
	for _, peer := range directories {
		directoryIDs[peer.Descriptor.ID] = true
		directoryOnions[strings.ToLower(peer.Descriptor.Onion)] = true
	}
	transitIDs, transitOnions := make(map[[32]byte]bool), make(map[string]bool)
	for _, relay := range transit {
		if err := relay.Verify(now); err != nil {
			return fmt.Errorf("invalid username transit descriptor: %w", err)
		}
		onion := strings.ToLower(relay.Onion)
		if transitIDs[relay.ID] || transitOnions[onion] {
			return errors.New("username transit relays must have distinct keys and onion services")
		}
		if directoryIDs[relay.ID] || directoryOnions[onion] {
			return errors.New("username transit relays must be independent from directory operators")
		}
		transitIDs[relay.ID], transitOnions[onion] = true, true
	}
	if len(transit) < 2 {
		return errors.New("username network requires two independent transit relays")
	}
	return nil
}

func (api *TkmNameAPI) isNetworkReady() bool {
	api.publicationMu.Lock()
	defer api.publicationMu.Unlock()
	return api.networkReady
}

func validatePinnedDirectorySet(peers []tkmnet.PinnedDirectoryPeer, now time.Time) error {
	if len(peers) < 2 {
		return errors.New("username network requires at least two pinned directory operators; additional operators are supported")
	}
	ids, onions, pins := make(map[[32]byte]bool), make(map[string]bool), make(map[[32]byte]bool)
	for _, peer := range peers {
		d := peer.Descriptor
		if err := d.Verify(now); err != nil {
			return fmt.Errorf("invalid username directory descriptor: %w", err)
		}
		fingerprint := sha256.Sum256(d.SigningPublicKey)
		if peer.SigningKeyPin == ([32]byte{}) || peer.SigningKeyPin != fingerprint {
			return errors.New("username directory signing key is not pinned")
		}
		if ids[d.ID] || onions[d.Onion] || pins[peer.SigningKeyPin] {
			return errors.New("username directory operators must have distinct keys and onion services")
		}
		ids[d.ID], onions[d.Onion], pins[peer.SigningKeyPin] = true, true, true
	}
	return nil
}

func (api *TkmNameAPI) usernameNetworkActive() bool {
	if api == nil || api.eth == nil || api.eth.BlockChain() == nil {
		return false
	}
	chain := api.eth.BlockChain()
	head := chain.CurrentHeader()
	return head != nil && head.Number != nil && chain.Config().IsUsernameNetworkActive(head.Number, head.Time)
}

func (api *TkmNameAPI) validateRecordStamp(record TkmNameRecord) error {
	chainID := api.eth.BlockChain().Config().ChainID.Uint64()
	if err := shield3wallet.VerifyUsernameBinding(record, chainID, time.Now().UTC()); err != nil {
		return err
	}
	payload, err := shield3wallet.DecodePaymentCode(record.PaymentCode, chainID)
	if err != nil {
		return err
	}
	state, err := api.eth.currentPrivacyState()
	if err != nil {
		return err
	}
	stamp, err := core.AntarticalStampForAddress(state, payload.Address)
	if err != nil {
		return err
	}
	if !stamp.Registered || stamp.Owner != payload.Owner || stamp.Commitment != payload.Stamp.Commitment {
		return errors.New("username registration requires a confirmed Shield3 address stamp")
	}
	return nil
}

// HandleNetworkPayload serves messages at this node's directory endpoint.
// It verifies all remote registrations and stores them against this node's
// current canonical head. It does not transmit registration data onward.
func (api *TkmNameAPI) HandleNetworkPayload(_ context.Context, service tkmnet.ServiceID, payload []byte) ([]byte, error) {
	if service != tkmnet.ServiceUsername {
		return nil, errors.New("unsupported TKMNet service")
	}
	if err := api.ready(); err != nil {
		return nil, err
	}
	if !api.usernameNetworkActive() {
		return nil, errors.New("TKMNet username service is not active at the canonical head")
	}
	if tkmnet.IsUsernameHealthRequest(payload) {
		return []byte{1}, nil
	}
	if tkmnet.IsUsernameCoverRequest(payload) {
		names := api.directory.Names(time.Now().UTC())
		selected := make([]string, 0, tkmnet.UsernameLookupFanout-1)
		for len(selected) < tkmnet.UsernameLookupFanout-1 && len(selected) < len(names) {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(names))))
			if err != nil {
				return nil, err
			}
			candidate := names[n.Int64()]
			duplicate := false
			for _, old := range selected {
				if old == candidate {
					duplicate = true
					break
				}
			}
			if !duplicate {
				selected = append(selected, candidate)
			}
		}
		return tkmnet.UsernameCoverReply(selected)
	}
	if encoded, ok := tkmnet.IsUsernameRegistrationRequest(payload); ok {
		var record TkmNameRecord
		if err := rlp.DecodeBytes(encoded, &record); err != nil {
			return nil, errors.New("invalid username registration message")
		}
		if err := api.syncCanonical(); err != nil {
			return nil, err
		}
		if err := api.validateRecordStamp(record); err != nil {
			return nil, err
		}
		head := api.eth.BlockChain().CurrentHeader()
		if head == nil || head.Number == nil {
			return nil, errors.New("canonical chain head is unavailable")
		}
		if err := api.directory.PutAt(record, time.Now().UTC(), head.Number.Uint64(), head.Hash()); err != nil {
			return nil, err
		}
		return []byte{1}, nil
	}
	name, err := tkmnet.DecodeUsernameQuery(payload)
	if err != nil {
		return nil, errors.New("unsupported TKMNet username request")
	}
	if err := api.syncCanonical(); err != nil {
		return nil, err
	}
	handle, err := shield3wallet.UsernameHandle(name, api.eth.BlockChain().Config().ChainID.Uint64())
	if err != nil {
		return nil, err
	}
	record, err := api.directory.Resolve(handle, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	encoded, err := rlp.EncodeToBytes(record)
	if err != nil {
		return nil, err
	}
	return tkmnet.UsernameRecordReply(encoded)
}

// syncNetworkDirectory publishes persisted local records after activation and
// checks every configured directory endpoint. It runs once per process unless
// a new local registration invalidates readiness.
func (api *TkmNameAPI) syncNetworkDirectory() error {
	api.publicationMu.Lock()
	defer api.publicationMu.Unlock()
	if api.networkReady {
		return nil
	}
	proxy, port, transit, peers := api.networkConfig()
	if proxy == "" || port == "" || len(transit) < 2 {
		return errors.New("Tor and two transit relays are required")
	}
	if err := validateUsernameNetworkConfig(proxy, port, transit, peers, time.Now()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	records := api.directory.Records(time.Now().UTC())
	if len(records) == 0 {
		if err := api.probeDirectoryPeers(ctx, proxy, port, transit, peers); err != nil {
			return err
		}
	} else {
		for _, record := range records {
			encoded, err := rlp.EncodeToBytes(record)
			if err != nil {
				return err
			}
			fingerprint := sha256.Sum256(encoded)
			if api.published[record.Username] == fingerprint {
				continue
			}
			if err := api.publishRecord(ctx, proxy, port, transit, peers, record); err != nil {
				return err
			}
			if api.published == nil {
				api.published = make(map[string][32]byte)
			}
			api.published[record.Username] = fingerprint
		}
	}
	api.networkReady = true
	return nil
}

func (api *TkmNameAPI) probeDirectoryPeers(ctx context.Context, proxy, port string, transit []tkmnet.Descriptor, peers []tkmnet.PinnedDirectoryPeer) error {
	type result struct{ err error }
	results := make(chan result, len(peers))
	for _, peer := range peers {
		peer := peer
		go func() {
			route, err := chooseUsernameRoute(transit, peer.Descriptor)
			if err == nil {
				var reply []byte
				reply, err = tkmnet.ExchangeService(ctx, proxy, port, route, tkmnet.ServiceUsername, tkmnet.UsernameHealthRequest())
				if err == nil && (len(reply) != 1 || reply[0] != 1) {
					err = errors.New("directory health check was rejected")
				}
			}
			results <- result{err: err}
		}()
	}
	var failures []error
	for i := 0; i < len(peers); i++ {
		if r := <-results; r.err != nil {
			failures = append(failures, r.err)
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("username directory health check failed: %w", errors.Join(failures...))
	}
	return nil
}

func (api *TkmNameAPI) publishRecord(ctx context.Context, proxy, port string, transit []tkmnet.Descriptor, peers []tkmnet.PinnedDirectoryPeer, record TkmNameRecord) error {
	encoded, err := rlp.EncodeToBytes(record)
	if err != nil {
		return err
	}
	payload, err := tkmnet.UsernameRegistrationRequest(encoded)
	if err != nil {
		return err
	}
	type result struct {
		id  [32]byte
		err error
	}
	results := make(chan result, len(peers))
	for _, peer := range peers {
		peer := peer
		go func() {
			route, err := chooseUsernameRoute(transit, peer.Descriptor)
			if err == nil {
				var reply []byte
				reply, err = tkmnet.ExchangeService(ctx, proxy, port, route, tkmnet.ServiceUsername, payload)
				if err == nil && (len(reply) != 1 || reply[0] != 1) {
					err = errors.New("directory rejected the signed binding")
				}
			}
			results <- result{id: peer.Descriptor.ID, err: err}
		}()
	}
	var failures []error
	for i := 0; i < len(peers); i++ {
		r := <-results
		if r.err != nil {
			failures = append(failures, fmt.Errorf("directory %x: %w", r.id[:6], r.err))
		}
	}
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	return nil
}

func chooseUsernamePeer(peers []tkmnet.PinnedDirectoryPeer) (tkmnet.PinnedDirectoryPeer, error) {
	if len(peers) == 0 {
		return tkmnet.PinnedDirectoryPeer{}, errors.New("username directory peer set is empty")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(peers))))
	if err != nil {
		return tkmnet.PinnedDirectoryPeer{}, err
	}
	return peers[n.Int64()], nil
}

func chooseUsernameRoute(transit []tkmnet.Descriptor, directory tkmnet.Descriptor) ([]tkmnet.Descriptor, error) {
	eligible := make([]tkmnet.Descriptor, 0, len(transit))
	seen := map[[32]byte]bool{directory.ID: true}
	for _, peer := range transit {
		if peer.ID != ([32]byte{}) && !seen[peer.ID] {
			seen[peer.ID] = true
			eligible = append(eligible, peer)
		}
	}
	if len(eligible) < 2 {
		return nil, errors.New("username route needs two distinct intermediate relays separate from the directory")
	}
	i, err := rand.Int(rand.Reader, big.NewInt(int64(len(eligible))))
	if err != nil {
		return nil, err
	}
	j, err := rand.Int(rand.Reader, big.NewInt(int64(len(eligible)-1)))
	if err != nil {
		return nil, err
	}
	first, second := int(i.Int64()), int(j.Int64())
	if second >= first {
		second++
	}
	route := []tkmnet.Descriptor{eligible[first], eligible[second], directory}
	for _, peer := range route {
		if err := peer.Verify(time.Now()); err != nil {
			return nil, fmt.Errorf("invalid username route descriptor: %w", err)
		}
	}
	return route, nil
}

func (api *TkmNameAPI) syncCanonical() error {
	if err := api.ready(); err != nil {
		return err
	}
	if api.eth == nil || api.eth.BlockChain() == nil {
		return errors.New("username directory is not attached to a chain")
	}
	chain := api.eth.BlockChain()
	head := chain.CurrentHeader()
	if head == nil || head.Number == nil {
		return errors.New("canonical chain head is unavailable")
	}
	return api.directory.SyncCanonical(shield3wallet.UsernameChainHead{
		BlockNumber: head.Number.Uint64(), BlockHash: head.Hash(),
	}, chain.GetCanonicalHash)
}

func (api *TkmNameAPI) ready() error {
	if api == nil {
		return errors.New("TKM username directory is unavailable")
	}
	if api.initErr != nil {
		return api.initErr
	}
	if api.directory == nil {
		return errors.New("TKM username directory is unavailable")
	}
	return nil
}
