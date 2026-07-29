package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	tkmInstitutionSuiteAddress = common.HexToAddress("0x43aeb055883863cfe40804e386bec801b4ca63ec")
	tkmInstitutionDeployTx     = common.HexToHash("0xcad679cf00644ec75008d79c2f104bde5584fe5e4f66a2987dd137d9730de12a")
	tkmInstitutionOwner        = common.HexToAddress("0x4441d6fEd0836B77a503e0B2788bfEd6FD8c23A8")
)

const tkmInstitutionDeployBlock uint64 = 0x3122

// InstitutionAPI provides transaction-data helpers and metadata for the deployed
// TKM institutional suite. It is an application-layer RPC service and does not
// mutate consensus state directly.
type InstitutionAPI struct{}

// NewInstitutionAPI creates the tkminstitution RPC service.
func NewInstitutionAPI() *InstitutionAPI { return &InstitutionAPI{} }

type InstitutionStatusResult struct {
	ContractAddress common.Address    `json:"contractAddress"`
	DeploymentTx    common.Hash       `json:"deploymentTx"`
	DeploymentBlock hexutil.Uint64    `json:"deploymentBlock"`
	Owner           common.Address    `json:"owner"`
	Modules         []string          `json:"modules"`
	Selectors       map[string]string `json:"selectors"`
	Statuses        map[string]uint8  `json:"statuses"`
	InvoiceStatuses map[string]uint8  `json:"invoiceStatuses"`
	EscrowStatuses  map[string]uint8  `json:"escrowStatuses"`
}

type InstitutionRegisterRequest struct {
	Admin               common.Address `json:"admin"`
	NameHash            common.Hash    `json:"nameHash"`
	InstitutionTypeHash common.Hash    `json:"institutionTypeHash"`
	RegistrationHash    common.Hash    `json:"registrationHash"`
	MetadataHash        common.Hash    `json:"metadataHash"`
	MetadataURI         string         `json:"metadataURI"`
}

type InstitutionProofRequest struct {
	InstitutionID common.Hash    `json:"institutionId"`
	SubjectHash   common.Hash    `json:"subjectHash"`
	RecordType    common.Hash    `json:"recordTypeHash"`
	ContentHash   common.Hash    `json:"contentHash"`
	ExpiresAt     hexutil.Uint64 `json:"expiresAt"`
	MetadataURI   string         `json:"metadataURI"`
}

type InstitutionInvoiceRequest struct {
	InstitutionID common.Hash    `json:"institutionId"`
	Payer         common.Address `json:"payer"`
	Amount        *hexutil.Big   `json:"amount"`
	InvoiceHash   common.Hash    `json:"invoiceHash"`
	DueAt         hexutil.Uint64 `json:"dueAt"`
	MetadataURI   string         `json:"metadataURI"`
}

type InstitutionDisclosureRequest struct {
	InstitutionID common.Hash `json:"institutionId"`
	CategoryHash  common.Hash `json:"categoryHash"`
	ContentHash   common.Hash `json:"contentHash"`
	PreviousHash  common.Hash `json:"previousHash"`
	MetadataURI   string      `json:"metadataURI"`
}

// Status returns the deployed suite address and supported modules/selectors.
func (api *InstitutionAPI) Status() InstitutionStatusResult {
	return InstitutionStatusResult{
		ContractAddress: tkmInstitutionSuiteAddress,
		DeploymentTx:    tkmInstitutionDeployTx,
		DeploymentBlock: hexutil.Uint64(tkmInstitutionDeployBlock),
		Owner:           tkmInstitutionOwner,
		Modules: []string{
			"InstitutionRegistry",
			"CredentialRegistry",
			"DocumentRegistry",
			"InvoiceSettlement",
			"EscrowVault",
			"ProcurementRegistry",
			"GrantRegistry",
			"AuditDisclosureRegistry",
		},
		Selectors: map[string]string{
			"registerInstitution":    "0x0e17aaf5",
			"setInstitutionStatus":   "0x15de4fa8",
			"rotateInstitutionAdmin": "0xb478e267",
			"issueDocument":          "0x2400157d",
			"issueCredential":        "0x88480808",
			"issueInvoice":           "0x6507bfea",
			"publishProcurement":     "0xcb983c7b",
			"publishGrant":           "0x5ff66c4b",
			"publishDisclosure":      "0x7b7652ca",
		},
		Statuses: map[string]uint8{
			"none":      0,
			"active":    1,
			"suspended": 2,
			"revoked":   3,
		},
		InvoiceStatuses: map[string]uint8{
			"none":      0,
			"issued":    1,
			"paid":      2,
			"cancelled": 3,
			"disputed":  4,
			"refunded":  5,
		},
		EscrowStatuses: map[string]uint8{
			"none":     0,
			"open":     1,
			"funded":   2,
			"released": 3,
			"refunded": 4,
			"disputed": 5,
			"resolved": 6,
		},
	}
}

// ContractAddress returns the canonical deployed institutional suite contract.
func (api *InstitutionAPI) ContractAddress() common.Address { return tkmInstitutionSuiteAddress }

// TextHash returns keccak256(utf8(text)) for UI and CLI callers.
func (api *InstitutionAPI) TextHash(text string) common.Hash {
	return crypto.Keccak256Hash([]byte(text))
}

// InstitutionID returns the exact ID used by TkmInstitutionalSuite.institutionIdFor.
func (api *InstitutionAPI) InstitutionID(admin common.Address, registrationHash common.Hash) common.Hash {
	payload := append([]byte("TKM_INSTITUTION"), admin.Bytes()...)
	payload = append(payload, registrationHash.Bytes()...)
	return crypto.Keccak256Hash(payload)
}

// RecordID returns the exact ID used by TkmInstitutionalSuite.recordIdFor.
func (api *InstitutionAPI) RecordID(namespace string, issuerID common.Hash, contentHash common.Hash) common.Hash {
	return crypto.Keccak256Hash(append(append(tkmInstitutionBytes32(namespace), issuerID.Bytes()...), contentHash.Bytes()...))
}

// RegisterInstitutionData returns ABI calldata for registerInstitution.
func (api *InstitutionAPI) RegisterInstitutionData(req InstitutionRegisterRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("registerInstitution(address,bytes32,bytes32,bytes32,bytes32,string)", "0x0e17aaf5", req.Admin, req.NameHash, req.InstitutionTypeHash, req.RegistrationHash, req.MetadataHash, req.MetadataURI)
}

// SetInstitutionStatusData returns ABI calldata for setInstitutionStatus.
func (api *InstitutionAPI) SetInstitutionStatusData(institutionID common.Hash, status uint8) (hexutil.Bytes, error) {
	return tkmInstitutionPack("setInstitutionStatus(bytes32,uint8)", "0x15de4fa8", institutionID, status)
}

// RotateInstitutionAdminData returns ABI calldata for rotateInstitutionAdmin.
func (api *InstitutionAPI) RotateInstitutionAdminData(institutionID common.Hash, newAdmin common.Address) (hexutil.Bytes, error) {
	return tkmInstitutionPack("rotateInstitutionAdmin(bytes32,address)", "0xb478e267", institutionID, newAdmin)
}

// IssueDocumentData returns ABI calldata for issueDocument.
func (api *InstitutionAPI) IssueDocumentData(req InstitutionProofRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("issueDocument(bytes32,bytes32,bytes32,bytes32,uint64,string)", "0x2400157d", req.InstitutionID, req.SubjectHash, req.RecordType, req.ContentHash, uint64(req.ExpiresAt), req.MetadataURI)
}

// IssueCredentialData returns ABI calldata for issueCredential.
func (api *InstitutionAPI) IssueCredentialData(req InstitutionProofRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("issueCredential(bytes32,bytes32,bytes32,bytes32,uint64,string)", "0x88480808", req.InstitutionID, req.SubjectHash, req.RecordType, req.ContentHash, uint64(req.ExpiresAt), req.MetadataURI)
}

// IssueInvoiceData returns ABI calldata for issueInvoice.
func (api *InstitutionAPI) IssueInvoiceData(req InstitutionInvoiceRequest) (hexutil.Bytes, error) {
	amount := (*big.Int)(req.Amount)
	if amount == nil {
		amount = new(big.Int)
	}
	return tkmInstitutionPack("issueInvoice(bytes32,address,uint256,bytes32,uint64,string)", "0x6507bfea", req.InstitutionID, req.Payer, amount, req.InvoiceHash, uint64(req.DueAt), req.MetadataURI)
}

// PublishProcurementData returns ABI calldata for publishProcurement.
func (api *InstitutionAPI) PublishProcurementData(req InstitutionProofRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("publishProcurement(bytes32,bytes32,bytes32,bytes32,string)", "0xcb983c7b", req.InstitutionID, req.SubjectHash, req.RecordType, req.ContentHash, req.MetadataURI)
}

// PublishGrantData returns ABI calldata for publishGrant.
func (api *InstitutionAPI) PublishGrantData(req InstitutionProofRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("publishGrant(bytes32,bytes32,bytes32,bytes32,string)", "0x5ff66c4b", req.InstitutionID, req.SubjectHash, req.RecordType, req.ContentHash, req.MetadataURI)
}

// PublishDisclosureData returns ABI calldata for publishDisclosure.
func (api *InstitutionAPI) PublishDisclosureData(req InstitutionDisclosureRequest) (hexutil.Bytes, error) {
	return tkmInstitutionPack("publishDisclosure(bytes32,bytes32,bytes32,bytes32,string)", "0x7b7652ca", req.InstitutionID, req.CategoryHash, req.ContentHash, req.PreviousHash, req.MetadataURI)
}

func tkmInstitutionBytes32(value string) []byte {
	out := make([]byte, 32)
	copy(out, []byte(value))
	return out
}

func tkmInstitutionPack(signature string, selectorHex string, values ...interface{}) (hexutil.Bytes, error) {
	arguments, err := tkmInstitutionArguments(signature)
	if err != nil {
		return nil, err
	}
	encoded, err := arguments.Pack(values...)
	if err != nil {
		return nil, err
	}
	selector := common.FromHex(selectorHex)
	return append(selector, encoded...), nil
}

func tkmInstitutionArguments(signature string) (abi.Arguments, error) {
	switch signature {
	case "registerInstitution(address,bytes32,bytes32,bytes32,bytes32,string)":
		return tkmInstitutionABIArgs("address", "bytes32", "bytes32", "bytes32", "bytes32", "string")
	case "setInstitutionStatus(bytes32,uint8)":
		return tkmInstitutionABIArgs("bytes32", "uint8")
	case "rotateInstitutionAdmin(bytes32,address)":
		return tkmInstitutionABIArgs("bytes32", "address")
	case "issueDocument(bytes32,bytes32,bytes32,bytes32,uint64,string)", "issueCredential(bytes32,bytes32,bytes32,bytes32,uint64,string)":
		return tkmInstitutionABIArgs("bytes32", "bytes32", "bytes32", "bytes32", "uint64", "string")
	case "issueInvoice(bytes32,address,uint256,bytes32,uint64,string)":
		return tkmInstitutionABIArgs("bytes32", "address", "uint256", "bytes32", "uint64", "string")
	case "publishProcurement(bytes32,bytes32,bytes32,bytes32,string)", "publishGrant(bytes32,bytes32,bytes32,bytes32,string)", "publishDisclosure(bytes32,bytes32,bytes32,bytes32,string)":
		return tkmInstitutionABIArgs("bytes32", "bytes32", "bytes32", "bytes32", "string")
	default:
		return nil, errUnknownInstitutionSignature
	}
}

var errUnknownInstitutionSignature = errors.New("unknown institutional ABI signature")

func tkmInstitutionABIArgs(types ...string) (abi.Arguments, error) {
	args := make(abi.Arguments, len(types))
	for i, typ := range types {
		parsed, err := abi.NewType(typ, "", nil)
		if err != nil {
			return nil, err
		}
		args[i] = abi.Argument{Type: parsed}
	}
	return args, nil
}
