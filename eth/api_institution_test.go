package eth

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestInstitutionAPIStatus(t *testing.T) {
	api := NewInstitutionAPI()
	status := api.Status()
	if status.ContractAddress != common.HexToAddress("0x43aeb055883863cfe40804e386bec801b4ca63ec") {
		t.Fatalf("contract address mismatch: %s", status.ContractAddress)
	}
	if status.DeploymentTx != common.HexToHash("0xcad679cf00644ec75008d79c2f104bde5584fe5e4f66a2987dd137d9730de12a") {
		t.Fatalf("deployment tx mismatch: %s", status.DeploymentTx)
	}
	if status.Selectors["registerInstitution"] != "0x0e17aaf5" || status.Selectors["issueInvoice"] != "0x6507bfea" {
		t.Fatalf("selector map missing expected methods: %#v", status.Selectors)
	}
	if len(status.Modules) != 8 {
		t.Fatalf("modules length = %d, want 8", len(status.Modules))
	}
}

func TestInstitutionAPIIDsMatchSolidityEncoding(t *testing.T) {
	api := NewInstitutionAPI()
	admin := common.HexToAddress("0x1111111111111111111111111111111111111111")
	registrationHash := common.HexToHash("0x07056792a6c7699a0a153a6851c882bfaf57a455e0246ac7178bd440efdc5f5f")
	if have, want := api.InstitutionID(admin, registrationHash), common.HexToHash("0x13e5afd93f4f176744cc55d47906c2a26e9a2de069f3ded944e1aa3ded08f39e"); have != want {
		t.Fatalf("InstitutionID = %s, want %s", have, want)
	}
	issuer := common.HexToHash("0xc06d9c5b991122f7c51a2cb89fc8efbf3e47e746c980f5afdbf2ac45f88aaf3d")
	content := common.HexToHash("0x1e109eb81780c8505fc75b12fda78c5737b7ccd70bfd04e22f4d22d598dd9e19")
	if have, want := api.RecordID("DOCUMENT", issuer, content), common.HexToHash("0xd3c7f38eaebdc78b6ef005d2dfae4dd11dc526ebd831068a025c5eb738adf15d"); have != want {
		t.Fatalf("RecordID = %s, want %s", have, want)
	}
}

func TestInstitutionAPICallData(t *testing.T) {
	api := NewInstitutionAPI()
	admin := common.HexToAddress("0x1111111111111111111111111111111111111111")
	nameHash := api.TextHash("Example University")
	typeHash := api.TextHash("education")
	registrationHash := api.TextHash("reg-1")
	metadataHash := api.TextHash("metadata-v1")
	data, err := api.RegisterInstitutionData(InstitutionRegisterRequest{
		Admin:               admin,
		NameHash:            nameHash,
		InstitutionTypeHash: typeHash,
		RegistrationHash:    registrationHash,
		MetadataHash:        metadataHash,
		MetadataURI:         "https://example.org/institution.json",
	})
	if err != nil {
		t.Fatalf("RegisterInstitutionData failed: %v", err)
	}
	if !bytes.HasPrefix(data, common.FromHex("0x0e17aaf5")) {
		t.Fatalf("register calldata selector mismatch: %x", []byte(data[:4]))
	}

	institutionID := api.InstitutionID(admin, registrationHash)
	amount := hexutil.Big(*big.NewInt(25_000))
	invoiceData, err := api.IssueInvoiceData(InstitutionInvoiceRequest{
		InstitutionID: institutionID,
		Payer:         common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Amount:        &amount,
		InvoiceHash:   api.TextHash("invoice-1"),
		DueAt:         hexutil.Uint64(12345),
		MetadataURI:   "ipfs://invoice-1",
	})
	if err != nil {
		t.Fatalf("IssueInvoiceData failed: %v", err)
	}
	if !bytes.HasPrefix(invoiceData, common.FromHex("0x6507bfea")) {
		t.Fatalf("invoice calldata selector mismatch: %x", []byte(invoiceData[:4]))
	}

	disclosureData, err := api.PublishDisclosureData(InstitutionDisclosureRequest{
		InstitutionID: institutionID,
		CategoryHash:  api.TextHash("audit"),
		ContentHash:   api.TextHash("audit-q3"),
		PreviousHash:  common.Hash{},
		MetadataURI:   "ipfs://audit-q3",
	})
	if err != nil {
		t.Fatalf("PublishDisclosureData failed: %v", err)
	}
	if !bytes.HasPrefix(disclosureData, common.FromHex("0x7b7652ca")) {
		t.Fatalf("disclosure calldata selector mismatch: %x", []byte(disclosureData[:4]))
	}
}
