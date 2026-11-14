package fiscal

import (
	"encoding/json"
	"os"
	"testing"
)

func example(t *testing.T) Invoice {
	t.Helper()
	raw, e := os.ReadFile("../../examples/document.json")
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Invoice json.RawMessage `json:"invoice"`
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	in, e := DecodeInvoice(doc.Invoice)
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func TestMoneyIsExactAndInvalidTotalsAreRejected(t *testing.T) {
	in := example(t)
	in.Items[0].Quantity = "3"
	in.Items[0].UnitPrice = "0.10"
	in.Items[0].Total = "0.30"
	in.Payments[0].Amount = "0.30"
	raw, _ := json.Marshal(in)
	got, e := DecodeInvoice(raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, tag := range got.Tags("nfe") {
		if tag.Tag == "ICMSTot" && tag.Values["vNF"] != "0.30" {
			t.Fatal("decimal arithmetic lost precision")
		}
	}
	in.Payments[0].Amount = "0.29"
	raw, _ = json.Marshal(in)
	if _, e = DecodeInvoice(raw); e == nil {
		t.Fatal("unbalanced payment accepted")
	}
	in.Payments[0].Amount = "0.30"
	in.Items[0].Total = "0.301"
	raw, _ = json.Marshal(in)
	if _, e = DecodeInvoice(raw); e == nil {
		t.Fatal("fractional cent accepted")
	}
}
func TestUnsupportedTaxCannotBeSilentlyDropped(t *testing.T) {
	in := example(t)
	in.Items[0].Taxes.ICMS.CST = "00"
	if in.ValidateSupportedTaxes() == nil {
		t.Fatal("ambiguous ICMS accepted")
	}
	in = example(t)
	in.Items[0].Taxes.ICMS.CSOSN = "201"
	if in.ValidateSupportedTaxes() == nil {
		t.Fatal("unsupported ST accepted")
	}
}

func TestIgnoredTaxFieldsRejected(t *testing.T) {
	for _, mutate := range []func(*Invoice){
		func(i *Invoice) { i.Items[0].Taxes.PIS.CSOSN = "102" },
		func(i *Invoice) { i.Items[0].Taxes.COFINS.Origin = 1 },
		func(i *Invoice) { i.Items[0].Taxes.PIS.CST = "07"; i.Items[0].Taxes.PIS.Amount = "10.00" },
		func(i *Invoice) { i.Items[0].Taxes.ICMS.Origin = 9 },
	} {
		in := example(t)
		mutate(&in)
		if in.ValidateSupportedTaxes() == nil {
			t.Fatal("silently ignored tax accepted")
		}
	}
}
func TestInvalidProductCodesRejected(t *testing.T) {
	for _, mutate := range []func(*Invoice){func(i *Invoice) { i.Items[0].NCM = "ABCDEFGH" }, func(i *Invoice) { i.Items[0].CFOP = "ABCD" }, func(i *Invoice) { i.Items[0].Unit = "" }} {
		in := example(t)
		mutate(&in)
		raw, _ := json.Marshal(in)
		if _, e := DecodeInvoice(raw); e == nil {
			t.Fatal("invalid product accepted")
		}
	}
}

func TestSimplifiedNationalInvoiceAndSmartDefaults(t *testing.T) {
	raw := []byte(`{
		"recipient": {
			"tax_id": "52998224725",
			"name": "CLIENTE CONSUMIDOR SP",
			"address": {
				"street": "AV PAULISTA",
				"number": "1000",
				"district": "BELA VISTA",
				"city_code": "3550308",
				"city": "SAO PAULO",
				"state": "SP",
				"postal_code": "01310100"
			}
		},
		"items": [
			{
				"description": "PRODUTO SIMPLIFICADO",
				"ncm": "61091000",
				"quantity": "2",
				"unit_price": "25.50"
			}
		]
	}`)
	in, err := DecodeInvoice(raw)
	if err != nil {
		t.Fatalf("simplified decode failed: %v", err)
	}
	if err := in.ValidateSupportedTaxes(); err != nil {
		t.Fatalf("simplified tax validation failed: %v", err)
	}
	companyAddr := &Address{
		Street:     "RUA TESTE",
		Number:     "100",
		District:   "CENTRO",
		CityCode:   "2927408",
		City:       "SALVADOR",
		State:      "BA",
		PostalCode: "40020000",
	}
	in.ApplyCompanyContext(companyAddr, "BA", 1, "nfe")
	if in.Destination != 2 {
		t.Fatalf("expected interstate destination=2 (BA -> SP), got %d", in.Destination)
	}
	if in.CityCode != "2927408" {
		t.Fatalf("expected city_code inherited from company (2927408), got %s", in.CityCode)
	}
	if in.Items[0].CFOP != "6102" {
		t.Fatalf("expected interstate CFOP 6102, got %s", in.Items[0].CFOP)
	}
	if in.Items[0].Total != "51.00" || len(in.Payments) != 1 || in.Payments[0].Amount != "51.00" {
		t.Fatalf("expected auto-calculated total and payment 51.00, got item=%s pay=%v", in.Items[0].Total, in.Payments)
	}
}

func TestReformaTributariaIBSCBSAndDIFAL(t *testing.T) {
	in := example(t)
	in.Items[0].Taxes.ICMSUFDest = &ICMSUFDest{
		InternalRate:   "18.00",
		InterstateRate: "12.00",
		FCPRate:        "2.00",
	}
	in.Items[0].Taxes.IBSCBS = &IBSCBS{
		CST:        "000",
		ClassTrib:  "000001",
		IBSUFRate:  "0.10",
		IBSMunRate: "0.05",
		CBSRate:    "0.90",
	}
	in.Items[0].Taxes.IS = &SelectiveTax{
		CST:       "01",
		ClassTrib: "000001",
		Base:      "10.00",
		Rate:      "5.00",
	}
	if err := in.ValidateSupportedTaxes(); err != nil {
		t.Fatalf("unexpected error validating RTC/DIFAL: %v", err)
	}
	tags := in.Tags("nfe")
	foundIBSCBS, foundIBSCBSTot, foundIS, foundDIFAL := false, false, false, false
	for _, tg := range tags {
		switch tg.Tag {
		case "IBSCBS":
			foundIBSCBS = true
		case "IBSCBSTot":
			foundIBSCBSTot = true
		case "IS":
			foundIS = true
		case "ICMSUFDest":
			foundDIFAL = true
			if tg.Values["vICMSUFDest"] != "0.60" || tg.Values["vFCPUFDest"] != "0.20" {
				t.Fatalf("unexpected DIFAL calculation: %v", tg.Values)
			}
		}
	}
	if !foundIBSCBS || !foundIBSCBSTot || !foundIS || !foundDIFAL {
		t.Fatalf("missing expected tags: ibscbs=%v tot=%v is=%v difal=%v", foundIBSCBS, foundIBSCBSTot, foundIS, foundDIFAL)
	}
}
