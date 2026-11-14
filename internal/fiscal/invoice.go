package fiscal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
)

// Public DTO: fiscal library tags never cross the public API boundary.
type Address struct {
	Street     string `json:"street"`
	Number     string `json:"number"`
	District   string `json:"district"`
	CityCode   string `json:"city_code"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
}

type Recipient struct {
	TaxID       string  `json:"tax_id"`
	Name        string  `json:"name"`
	IEIndicator int     `json:"ie_indicator"`
	IE          string  `json:"ie,omitempty"`
	Email       string  `json:"email,omitempty"`
	Address     Address `json:"address"`
}

type Tax struct {
	CST                string `json:"cst,omitempty"`
	CSOSN              string `json:"csosn,omitempty"`
	Origin             int    `json:"origin,omitempty"`
	Base               string `json:"base,omitempty"`
	Rate               string `json:"rate,omitempty"`
	Amount             string `json:"amount,omitempty"`
	ReductionRate      string `json:"reduction_rate,omitempty"`
	MVA                string `json:"mva,omitempty"`
	BaseST             string `json:"base_st,omitempty"`
	RateST             string `json:"rate_st,omitempty"`
	AmountST           string `json:"amount_st,omitempty"`
	BaseSTRetained     string `json:"base_st_retained,omitempty"`
	AmountSTRetained   string `json:"amount_st_retained,omitempty"`
	STRate             string `json:"st_rate,omitempty"`
	CreditRate         string `json:"credit_rate,omitempty"`
	CreditAmount       string `json:"credit_amount,omitempty"`
	DesonerationAmount string `json:"desoneration_amount,omitempty"`
	DesonerationReason int    `json:"desoneration_reason,omitempty"`
	Enquadramento      string `json:"enquadramento,omitempty"`
}

type ICMSUFDest struct {
	Base           string `json:"base,omitempty"`
	FCPBase        string `json:"fcp_base,omitempty"`
	FCPRate        string `json:"fcp_rate,omitempty"`
	InternalRate   string `json:"internal_rate"`
	InterstateRate string `json:"interstate_rate"`
	ShareRate      string `json:"share_rate,omitempty"`
	FCPAmount      string `json:"fcp_amount,omitempty"`
	DestAmount     string `json:"dest_amount,omitempty"`
	OriginAmount   string `json:"origin_amount,omitempty"`
}

type IBSCBS struct {
	CST          string `json:"cst"`
	ClassTrib    string `json:"c_class_trib"`
	Base         string `json:"base,omitempty"`
	IBSUFRate    string `json:"ibs_uf_rate,omitempty"`
	IBSUFAmount  string `json:"ibs_uf_amount,omitempty"`
	IBSMunRate   string `json:"ibs_mun_rate,omitempty"`
	IBSMunAmount string `json:"ibs_mun_amount,omitempty"`
	CBSRate      string `json:"cbs_rate,omitempty"`
	CBSAmount    string `json:"cbs_amount,omitempty"`
}

type SelectiveTax struct {
	CST       string `json:"cst"`
	ClassTrib string `json:"c_class_trib"`
	Base      string `json:"base"`
	Rate      string `json:"rate"`
	Amount    string `json:"amount,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Quantity  string `json:"quantity,omitempty"`
}

type Taxes struct {
	ICMS       Tax           `json:"icms"`
	ICMSUFDest *ICMSUFDest   `json:"icms_uf_dest,omitempty"`
	IPI        *Tax          `json:"ipi,omitempty"`
	PIS        Tax           `json:"pis"`
	COFINS     Tax           `json:"cofins"`
	IBSCBS     *IBSCBS       `json:"ibs_cbs,omitempty"`
	IS         *SelectiveTax `json:"is,omitempty"`
}

type Item struct {
	SKU            string `json:"sku"`
	Description    string `json:"description"`
	NCM            string `json:"ncm"`
	CEST           string `json:"cest,omitempty"`
	CFOP           string `json:"cfop"`
	GTIN           string `json:"gtin,omitempty"`
	Unit           string `json:"unit"`
	Quantity       string `json:"quantity"`
	UnitPrice      string `json:"unit_price"`
	Total          string `json:"total"`
	Discount       string `json:"discount,omitempty"`
	Freight        string `json:"freight,omitempty"`
	Insurance      string `json:"insurance,omitempty"`
	OtherExpenses  string `json:"other_expenses,omitempty"`
	AdditionalInfo string `json:"additional_info,omitempty"`
	Taxes          Taxes  `json:"taxes"`

	autoCFOP  bool
	autoTaxes bool
}

type Payment struct {
	Method string `json:"method"`
	Amount string `json:"amount"`
}

type Invoice struct {
	Nature         string    `json:"nature"`
	Purpose        int       `json:"purpose,omitempty"`
	Direction      int       `json:"direction"`
	Destination    int       `json:"destination"`
	CityCode       string    `json:"city_code"`
	FinalConsumer  bool      `json:"final_consumer"`
	Presence       int       `json:"presence"`
	IssuerAddress  Address   `json:"issuer_address"`
	Recipient      Recipient `json:"recipient"`
	Items          []Item    `json:"items"`
	Payments       []Payment `json:"payments"`
	FreightMode    int       `json:"freight_mode"`
	AdditionalInfo string    `json:"additional_info,omitempty"`
	ReferencedKeys []string  `json:"referenced_keys,omitempty"`

	autoDestination bool
	autoCityCode    bool
}

type Tag struct {
	Tag    string         `json:"tag"`
	Values map[string]any `json:"values"`
}

var ncmPattern = regexp.MustCompile(`^[0-9]{8}$`)
var cfopPattern = regexp.MustCompile(`^[0-9]{4}$`)
var cestPattern = regexp.MustCompile(`^[0-9]{7}$`)
var keyPattern = regexp.MustCompile(`^[0-9A-Z]{44}$`)

var money = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)
var decimal = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,10})?$`)

func amount(s string) (*big.Rat, error) {
	if !decimal.MatchString(s) {
		return nil, errors.New("invalid decimal")
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return v, nil
}

func moneyVal(s string) (*big.Rat, error) {
	if s == "" {
		return new(big.Rat), nil
	}
	if !money.MatchString(s) {
		return nil, errors.New("valor monetário deve ter no máximo duas casas decimais")
	}
	return amount(s)
}

func DecodeInvoice(raw []byte) (Invoice, error) {
	var in Invoice
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&in); e != nil {
		return in, errors.New("invoice possui campos inválidos")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return in, errors.New("invoice deve conter somente um objeto JSON")
	}

	var rawMap map[string]json.RawMessage
	_ = json.Unmarshal(raw, &rawMap)
	var rawItems []map[string]json.RawMessage
	if itemsRaw, ok := rawMap["items"]; ok {
		_ = json.Unmarshal(itemsRaw, &rawItems)
	}

	// Smart Defaults for Simplified National Generation
	if strings.TrimSpace(in.Nature) == "" {
		if _, present := rawMap["nature"]; !present {
			in.Nature = "VENDA DE MERCADORIA"
		}
	}
	if _, present := rawMap["direction"]; !present {
		in.Direction = 1
	}
	if _, present := rawMap["presence"]; !present {
		in.Presence = 1
	}
	if _, present := rawMap["freight_mode"]; !present {
		in.FreightMode = 9
	}
	if in.Purpose == 0 {
		in.Purpose = 1
	}
	if _, present := rawMap["destination"]; !present {
		in.autoDestination = true
		if in.IssuerAddress.State != "" && in.Recipient.Address.State != "" {
			if in.Recipient.Address.State == "EX" {
				in.Destination = 3
			} else if in.Recipient.Address.State != in.IssuerAddress.State {
				in.Destination = 2
			} else {
				in.Destination = 1
			}
		} else {
			in.Destination = 1
		}
	}
	if in.CityCode == "" {
		if _, present := rawMap["city_code"]; !present {
			in.autoCityCode = true
			if in.IssuerAddress.CityCode != "" {
				in.CityCode = in.IssuerAddress.CityCode
			} else if in.Recipient.Address.CityCode != "" {
				in.CityCode = in.Recipient.Address.CityCode
			} else {
				in.CityCode = "0000000"
			}
		}
	}
	if _, present := rawMap["final_consumer"]; !present {
		if len(in.Recipient.TaxID) == 11 || in.Recipient.IEIndicator == 9 || in.Recipient.TaxID == "" {
			in.FinalConsumer = true
		}
	}
	if in.Recipient.IEIndicator == 0 {
		if in.Recipient.IE != "" && in.Recipient.IE != "ISENTO" {
			in.Recipient.IEIndicator = 1
		} else if in.Recipient.IE == "ISENTO" {
			in.Recipient.IEIndicator = 2
		} else {
			in.Recipient.IEIndicator = 9
		}
	}

	if len(in.Items) == 0 || len(in.Items) > 990 || in.Nature == "" || in.CityCode == "" || in.Destination < 1 || in.Destination > 3 || in.Direction < 0 || in.Direction > 1 || in.Purpose < 1 || in.Purpose > 4 {
		return in, errors.New("invoice incompleta")
	}
	for _, rk := range in.ReferencedKeys {
		if !keyPattern.MatchString(rk) {
			return in, errors.New("chave referenciada inválida")
		}
	}

	productsTotal := new(big.Rat)
	netInvoiceTotal := new(big.Rat)

	for idx := range in.Items {
		it := &in.Items[idx]
		var itemMap map[string]json.RawMessage
		if idx < len(rawItems) {
			itemMap = rawItems[idx]
		}
		if it.Unit == "" {
			if _, present := itemMap["unit"]; !present {
				it.Unit = "UN"
			}
		}
		if it.SKU == "" {
			if _, present := itemMap["sku"]; !present {
				it.SKU = fmt.Sprintf("ITEM-%03d", idx+1)
			}
		}
		if it.CFOP == "" {
			if _, present := itemMap["cfop"]; !present {
				it.autoCFOP = true
				if in.Destination == 2 {
					it.CFOP = "6102"
				} else {
					it.CFOP = "5102"
				}
			}
		}
		if it.Taxes.ICMS.CST == "" && it.Taxes.ICMS.CSOSN == "" && it.Taxes.PIS.CST == "" && it.Taxes.COFINS.CST == "" {
			if _, present := itemMap["taxes"]; !present {
				it.autoTaxes = true
				it.Taxes.ICMS = Tax{Origin: 0, CSOSN: "102"}
				it.Taxes.PIS = Tax{CST: "07"}
				it.Taxes.COFINS = Tax{CST: "07"}
			}
		}

		q, e := amount(it.Quantity)
		if e != nil || q.Sign() <= 0 {
			return in, errors.New("quantidade inválida")
		}
		p, e := amount(it.UnitPrice)
		if e != nil {
			return in, e
		}
		calculated := new(big.Rat).Mul(q, p)
		if it.Total == "" {
			if _, present := itemMap["total"]; !present {
				it.Total = calculated.FloatString(2)
			}
		}
		total, e := amount(it.Total)
		if e != nil {
			return in, e
		}
		if !money.MatchString(it.Total) {
			return in, errors.New("total deve ter no máximo duas casas decimais")
		}
		if calculated.FloatString(2) != total.FloatString(2) {
			return in, errors.New("total do item difere de quantidade vezes preço")
		}
		productsTotal.Add(productsTotal, total)
		netInvoiceTotal.Add(netInvoiceTotal, total)

		for _, optField := range []struct {
			val string
			sub bool
		}{{it.Discount, true}, {it.Freight, false}, {it.Insurance, false}, {it.OtherExpenses, false}} {
			if optField.val != "" {
				v, err := moneyVal(optField.val)
				if err != nil {
					return in, err
				}
				if optField.sub {
					netInvoiceTotal.Sub(netInvoiceTotal, v)
				} else {
					netInvoiceTotal.Add(netInvoiceTotal, v)
				}
			}
		}
		if it.Taxes.IPI != nil && it.Taxes.IPI.Amount != "" {
			v, err := moneyVal(it.Taxes.IPI.Amount)
			if err != nil {
				return in, err
			}
			netInvoiceTotal.Add(netInvoiceTotal, v)
		}
		if it.Taxes.ICMS.AmountST != "" {
			v, err := moneyVal(it.Taxes.ICMS.AmountST)
			if err != nil {
				return in, err
			}
			netInvoiceTotal.Add(netInvoiceTotal, v)
		}

		if it.SKU == "" || it.Description == "" || !ncmPattern.MatchString(it.NCM) || !cfopPattern.MatchString(it.CFOP) || it.Unit == "" {
			return in, errors.New("produto incompleto")
		}
		if it.CEST != "" && !cestPattern.MatchString(it.CEST) {
			return in, errors.New("CEST inválido")
		}
		for _, tax := range []Tax{it.Taxes.ICMS, it.Taxes.PIS, it.Taxes.COFINS} {
			for _, v := range []string{tax.Base, tax.Rate, tax.Amount, tax.ReductionRate, tax.MVA, tax.BaseST, tax.RateST, tax.AmountST, tax.BaseSTRetained, tax.AmountSTRetained, tax.STRate, tax.CreditRate, tax.CreditAmount, tax.DesonerationAmount} {
				if v != "" {
					if _, e := amount(v); e != nil {
						return in, e
					}
				}
			}
		}
	}

	if len(in.Payments) == 0 {
		if _, present := rawMap["payments"]; !present {
			in.Payments = []Payment{{Method: "01", Amount: netInvoiceTotal.FloatString(2)}}
		}
	} else if len(in.Payments) == 1 && in.Payments[0].Amount == "" {
		in.Payments[0].Amount = netInvoiceTotal.FloatString(2)
	}
	if len(in.Payments) == 0 || len(in.Payments) > 100 {
		return in, errors.New("invoice incompleta")
	}

	paymentsTotal := new(big.Rat)
	for _, p := range in.Payments {
		v, e := amount(p.Amount)
		if e != nil || !money.MatchString(p.Amount) {
			return in, errors.New("pagamento inválido")
		}
		paymentsTotal.Add(paymentsTotal, v)
	}
	if netInvoiceTotal.Cmp(paymentsTotal) != 0 && productsTotal.Cmp(paymentsTotal) != 0 {
		return in, errors.New("pagamentos devem totalizar o valor dos produtos neste contrato")
	}

	return in, nil
}

// ApplyCompanyContext enriches simplified invoices with the emitter company's UF, city_code and CRT.
func (in *Invoice) ApplyCompanyContext(companyAddress *Address, companyUF string, companyCRT int, kind string) {
	if companyAddress != nil {
		in.IssuerAddress = *companyAddress
		if in.autoCityCode || in.CityCode == "0000000" || in.CityCode == "" {
			in.CityCode = companyAddress.CityCode
		}
	}
	if in.autoDestination && companyUF != "" {
		if kind == "nfce" {
			in.Destination = 1
		} else if in.Recipient.Address.State == "EX" {
			in.Destination = 3
		} else if in.Recipient.Address.State != "" && in.Recipient.Address.State != companyUF {
			in.Destination = 2
		} else {
			in.Destination = 1
		}
	}
	for idx := range in.Items {
		it := &in.Items[idx]
		if it.autoCFOP {
			if in.Destination == 2 && kind != "nfce" {
				it.CFOP = "6102"
			} else {
				it.CFOP = "5102"
			}
		}
		if it.autoTaxes && companyCRT == 3 {
			// For Regime Normal (CRT 3), default simplified items to CST 00 (zeroed base/rate) or CST 41
			it.Taxes.ICMS = Tax{Origin: 0, CST: "41"}
			it.Taxes.PIS = Tax{CST: "07"}
			it.Taxes.COFINS = Tax{CST: "07"}
		}
	}
}

func address(a Address) map[string]any {
	return map[string]any{"xLgr": a.Street, "nro": a.Number, "xBairro": a.District, "cMun": a.CityCode, "xMun": a.City, "UF": a.State, "CEP": a.PostalCode, "cPais": "1058", "xPais": "BRASIL"}
}

func (in Invoice) Tags(kind string) []Tag {
	tags := []Tag{}
	add := func(name string, values map[string]any) { tags = append(tags, Tag{name, values}) }
	consumer := 0
	if in.FinalConsumer || kind == "nfce" {
		consumer = 1
	}
	printType := 1
	if kind == "nfce" {
		printType = 4
	}
	purpose := in.Purpose
	if purpose == 0 {
		purpose = 1
	}
	add("ide", map[string]any{"natOp": in.Nature, "tpNF": in.Direction, "idDest": in.Destination, "cMunFG": in.CityCode, "tpImp": printType, "finNFe": purpose, "indFinal": consumer, "indPres": in.Presence})
	for _, rk := range in.ReferencedKeys {
		add("refNFe", map[string]any{"refNFe": rk})
	}
	add("enderEmit", address(in.IssuerAddress))
	if kind != "nfce" || in.Recipient.TaxID != "" || in.Recipient.Name != "" {
		recipient := map[string]any{"indIEDest": in.Recipient.IEIndicator}
		if in.Recipient.Name != "" {
			recipient["xNome"] = in.Recipient.Name
		}
		if len(in.Recipient.TaxID) == 11 {
			recipient["CPF"] = in.Recipient.TaxID
		} else if in.Recipient.TaxID != "" {
			recipient["CNPJ"] = in.Recipient.TaxID
		}
		if in.Recipient.IE != "" {
			recipient["IE"] = in.Recipient.IE
		}
		if in.Recipient.Email != "" {
			recipient["email"] = in.Recipient.Email
		}
		add("dest", recipient)
		if in.Recipient.Address.Street != "" {
			add("enderDest", address(in.Recipient.Address))
		}
	}

	totals := map[string]*big.Rat{}
	for _, k := range []string{"vBC", "vICMS", "vICMSDeson", "vFCP", "vBCST", "vST", "vFCPST", "vFCPSTRet", "vProd", "vFrete", "vSeg", "vDesc", "vII", "vIPI", "vIPIDevol", "vPIS", "vCOFINS", "vOutro", "vNF", "vTotTrib", "vFCPUFDest", "vICMSUFDest", "vICMSUFRemet"} {
		totals[k] = new(big.Rat)
	}
	hasIBSCBS := false
	ibsTot := map[string]*big.Rat{
		"vBCIBSCBS": new(big.Rat),
		"vIBSUF":    new(big.Rat),
		"vIBSMun":   new(big.Rat),
		"vIBS":      new(big.Rat),
		"vCBS":      new(big.Rat),
	}
	hasIS := false
	vISTot := new(big.Rat)

	for i, it := range in.Items {
		item := i + 1
		gtin := it.GTIN
		if gtin == "" {
			gtin = "SEM GTIN"
		}
		prod := map[string]any{
			"item":     item,
			"cProd":    it.SKU,
			"cEAN":     gtin,
			"xProd":    it.Description,
			"NCM":      it.NCM,
			"CFOP":     it.CFOP,
			"uCom":     it.Unit,
			"qCom":     it.Quantity,
			"vUnCom":   it.UnitPrice,
			"vProd":    it.Total,
			"cEANTrib": gtin,
			"uTrib":    it.Unit,
			"qTrib":    it.Quantity,
			"vUnTrib":  it.UnitPrice,
			"indTot":   1,
		}
		for _, opt := range []struct{ k, v, tot string }{
			{"vDesc", it.Discount, "vDesc"},
			{"vFrete", it.Freight, "vFrete"},
			{"vSeg", it.Insurance, "vSeg"},
			{"vOutro", it.OtherExpenses, "vOutro"},
		} {
			if opt.v != "" {
				prod[opt.k] = opt.v
				if n, err := amount(opt.v); err == nil {
					totals[opt.tot].Add(totals[opt.tot], n)
				}
			}
		}
		add("prod", prod)
		if it.CEST != "" {
			add("CEST", map[string]any{"item": item, "CEST": it.CEST})
		}
		if it.AdditionalInfo != "" {
			add("infAdProd", map[string]any{"item": item, "vAdProd": it.AdditionalInfo})
		}
		add("imposto", map[string]any{"item": item})
		value, _ := amount(it.Total)
		totals["vProd"].Add(totals["vProd"], value)

		icms := map[string]any{"item": item, "orig": it.Taxes.ICMS.Origin}
		if it.Taxes.ICMS.CSOSN != "" {
			icms["CSOSN"] = it.Taxes.ICMS.CSOSN
			if it.Taxes.ICMS.CreditRate != "" {
				icms["pCredSN"] = it.Taxes.ICMS.CreditRate
			}
			if it.Taxes.ICMS.CreditAmount != "" {
				icms["vCredICMSSN"] = it.Taxes.ICMS.CreditAmount
			}
			if it.Taxes.ICMS.Base != "" {
				icms["modBC"] = 3
				icms["vBC"] = it.Taxes.ICMS.Base
				icms["pICMS"] = it.Taxes.ICMS.Rate
				icms["vICMS"] = it.Taxes.ICMS.Amount
			}
			if it.Taxes.ICMS.BaseST != "" {
				icms["modBCST"] = 4
				if it.Taxes.ICMS.MVA != "" {
					icms["pMVAST"] = it.Taxes.ICMS.MVA
				}
				icms["vBCST"] = it.Taxes.ICMS.BaseST
				icms["pICMSST"] = it.Taxes.ICMS.RateST
				icms["vICMSST"] = it.Taxes.ICMS.AmountST
				if n, err := amount(it.Taxes.ICMS.BaseST); err == nil {
					totals["vBCST"].Add(totals["vBCST"], n)
				}
				if n, err := amount(it.Taxes.ICMS.AmountST); err == nil {
					totals["vST"].Add(totals["vST"], n)
				}
			}
			add("ICMSSN", icms)
		} else {
			icms["CST"] = it.Taxes.ICMS.CST
			if it.Taxes.ICMS.Base != "" {
				icms["modBC"] = 3
				icms["vBC"] = it.Taxes.ICMS.Base
				icms["pICMS"] = it.Taxes.ICMS.Rate
				icms["vICMS"] = it.Taxes.ICMS.Amount
			}
			if it.Taxes.ICMS.ReductionRate != "" {
				icms["pRedBC"] = it.Taxes.ICMS.ReductionRate
			}
			if it.Taxes.ICMS.BaseST != "" {
				icms["modBCST"] = 4
				if it.Taxes.ICMS.MVA != "" {
					icms["pMVAST"] = it.Taxes.ICMS.MVA
				}
				icms["vBCST"] = it.Taxes.ICMS.BaseST
				icms["pICMSST"] = it.Taxes.ICMS.RateST
				icms["vICMSST"] = it.Taxes.ICMS.AmountST
				if n, err := amount(it.Taxes.ICMS.BaseST); err == nil {
					totals["vBCST"].Add(totals["vBCST"], n)
				}
				if n, err := amount(it.Taxes.ICMS.AmountST); err == nil {
					totals["vST"].Add(totals["vST"], n)
				}
			}
			if it.Taxes.ICMS.BaseSTRetained != "" {
				icms["vBCSTRet"] = it.Taxes.ICMS.BaseSTRetained
				icms["vICMSSTRet"] = it.Taxes.ICMS.AmountSTRetained
				if it.Taxes.ICMS.STRate != "" {
					icms["pST"] = it.Taxes.ICMS.STRate
				}
			}
			if it.Taxes.ICMS.DesonerationAmount != "" {
				icms["vICMSDeson"] = it.Taxes.ICMS.DesonerationAmount
				icms["motDesICMS"] = it.Taxes.ICMS.DesonerationReason
				if n, err := amount(it.Taxes.ICMS.DesonerationAmount); err == nil {
					totals["vICMSDeson"].Add(totals["vICMSDeson"], n)
				}
			}
			add("ICMS", icms)
			for _, v := range []struct{ k, s string }{{"vBC", it.Taxes.ICMS.Base}, {"vICMS", it.Taxes.ICMS.Amount}} {
				if v.s != "" {
					n, _ := amount(v.s)
					totals[v.k].Add(totals[v.k], n)
				}
			}
		}

		// DIFAL (ICMSUFDest) for interstate sales to final consumer
		if it.Taxes.ICMSUFDest != nil {
			d := it.Taxes.ICMSUFDest
			bc := d.Base
			if bc == "" {
				bc = it.Total
			}
			bcRat, _ := amount(bc)
			intRat, _ := amount(d.InternalRate)
			interRat, _ := amount(d.InterstateRate)
			destAmt := d.DestAmount
			if destAmt == "" && bcRat != nil && intRat != nil && interRat != nil {
				diff := new(big.Rat).Sub(intRat, interRat)
				Hundred := big.NewRat(100, 1)
				destAmt = new(big.Rat).Quo(new(big.Rat).Mul(bcRat, diff), Hundred).FloatString(2)
			}
			fcpAmt := d.FCPAmount
			if fcpAmt == "" && d.FCPRate != "" && bcRat != nil {
				if fcpRat, err := amount(d.FCPRate); err == nil {
					fcpAmt = new(big.Rat).Quo(new(big.Rat).Mul(bcRat, fcpRat), big.NewRat(100, 1)).FloatString(2)
				}
			}
			if fcpAmt == "" {
				fcpAmt = "0.00"
			}
			share := d.ShareRate
			if share == "" {
				share = "100.00"
			}
			origAmt := d.OriginAmount
			if origAmt == "" {
				origAmt = "0.00"
			}
			ufDest := map[string]any{
				"item":           item,
				"vBCUFDest":      bc,
				"vBCFCPUFDest":   bc,
				"pFCPUFDest":      coalesceStr(d.FCPRate, "0.00"),
				"pICMSUFDest":    d.InternalRate,
				"pICMSInter":     d.InterstateRate,
				"pICMSInterPart": share,
				"vFCPUFDest":     fcpAmt,
				"vICMSUFDest":    destAmt,
				"vICMSUFRemet":   origAmt,
			}
			add("ICMSUFDest", ufDest)
			if n, err := amount(fcpAmt); err == nil {
				totals["vFCPUFDest"].Add(totals["vFCPUFDest"], n)
			}
			if n, err := amount(destAmt); err == nil {
				totals["vICMSUFDest"].Add(totals["vICMSUFDest"], n)
			}
			if n, err := amount(origAmt); err == nil {
				totals["vICMSUFRemet"].Add(totals["vICMSUFRemet"], n)
			}
		}

		// IPI
		if it.Taxes.IPI != nil {
			enq := it.Taxes.IPI.Enquadramento
			if enq == "" {
				enq = "999"
			}
			ipi := map[string]any{"item": item, "cEnq": enq, "CST": it.Taxes.IPI.CST}
			if it.Taxes.IPI.Base != "" {
				ipi["vBC"] = it.Taxes.IPI.Base
			}
			if it.Taxes.IPI.Rate != "" {
				ipi["pIPI"] = it.Taxes.IPI.Rate
			}
			if it.Taxes.IPI.Amount != "" {
				ipi["vIPI"] = it.Taxes.IPI.Amount
				if n, err := amount(it.Taxes.IPI.Amount); err == nil {
					totals["vIPI"].Add(totals["vIPI"], n)
				}
			}
			add("IPI", ipi)
		}

		for _, t := range []struct {
			name string
			tax  Tax
		}{{"PIS", it.Taxes.PIS}, {"COFINS", it.Taxes.COFINS}} {
			vals := map[string]any{"item": item, "CST": t.tax.CST}
			if t.tax.Base != "" {
				vals["vBC"] = t.tax.Base
			} else if t.tax.CST >= "49" {
				vals["vBC"] = "0.00"
			}
			if t.tax.Rate != "" {
				vals["p"+t.name] = t.tax.Rate
			} else if t.tax.CST >= "49" {
				vals["p"+t.name] = "0.00"
			}
			if t.tax.Amount != "" {
				vals["v"+t.name] = t.tax.Amount
				n, _ := amount(t.tax.Amount)
				totals["v"+t.name].Add(totals["v"+t.name], n)
			} else if t.tax.CST >= "49" {
				vals["v"+t.name] = "0.00"
			}
			add(t.name, vals)
		}

		// Reforma Tributária: Imposto Seletivo (IS)
		if it.Taxes.IS != nil {
			hasIS = true
			isTax := it.Taxes.IS
			bcRat, _ := amount(isTax.Base)
			rateRat, _ := amount(isTax.Rate)
			amtStr := isTax.Amount
			if amtStr == "" && bcRat != nil && rateRat != nil {
				amtStr = new(big.Rat).Quo(new(big.Rat).Mul(bcRat, rateRat), big.NewRat(100, 1)).FloatString(2)
			}
			if n, err := amount(amtStr); err == nil {
				vISTot.Add(vISTot, n)
			}
			add("IS", map[string]any{
				"item":         item,
				"CSTIS":        isTax.CST,
				"cClassTribIS": isTax.ClassTrib,
				"vBCIS":        isTax.Base,
				"pIS":          isTax.Rate,
				"vIS":          amtStr,
				"uTrib":        coalesceStr(isTax.Unit, it.Unit),
				"qTrib":        coalesceStr(isTax.Quantity, it.Quantity),
			})
		}

		// Reforma Tributária: IBS e CBS
		if it.Taxes.IBSCBS != nil {
			hasIBSCBS = true
			rtc := it.Taxes.IBSCBS
			bc := rtc.Base
			if bc == "" {
				bc = it.Total
			}
			bcRat, _ := amount(bc)
			hundred := big.NewRat(100, 1)

			calcTax := func(rateStr, amtStr string) (string, *big.Rat) {
				if amtStr != "" {
					r, _ := amount(amtStr)
					if r == nil {
						r = new(big.Rat)
					}
					return amtStr, r
				}
				if rateStr != "" && bcRat != nil {
					if rateRat, err := amount(rateStr); err == nil {
						v := new(big.Rat).Quo(new(big.Rat).Mul(bcRat, rateRat), hundred)
						return v.FloatString(2), v
					}
				}
				return "0.00", new(big.Rat)
			}

			vUFStr, vUFRat := calcTax(rtc.IBSUFRate, rtc.IBSUFAmount)
			vMunStr, vMunRat := calcTax(rtc.IBSMunRate, rtc.IBSMunAmount)
			vCBSStr, vCBSRat := calcTax(rtc.CBSRate, rtc.CBSAmount)
			vIBSRat := new(big.Rat).Add(vUFRat, vMunRat)

			if bcRat != nil {
				ibsTot["vBCIBSCBS"].Add(ibsTot["vBCIBSCBS"], bcRat)
			}
			ibsTot["vIBSUF"].Add(ibsTot["vIBSUF"], vUFRat)
			ibsTot["vIBSMun"].Add(ibsTot["vIBSMun"], vMunRat)
			ibsTot["vIBS"].Add(ibsTot["vIBS"], vIBSRat)
			ibsTot["vCBS"].Add(ibsTot["vCBS"], vCBSRat)

			add("IBSCBS", map[string]any{
				"item":             item,
				"CST":              rtc.CST,
				"cClassTrib":       rtc.ClassTrib,
				"vBC":              bc,
				"vIBS":             vIBSRat.FloatString(2),
				"gIBSUF_pIBSUF":    coalesceStr(rtc.IBSUFRate, "0.00"),
				"gIBSUF_vIBSUF":    vUFStr,
				"gIBSMun_pIBSMun":  coalesceStr(rtc.IBSMunRate, "0.00"),
				"gIBSMun_vIBSMun":  vMunStr,
				"gCBS_pCBS":        coalesceStr(rtc.CBSRate, "0.00"),
				"gCBS_vCBS":        vCBSStr,
			})
		}
	}

	vNF := new(big.Rat).Set(totals["vProd"])
	vNF.Sub(vNF, totals["vDesc"])
	vNF.Add(vNF, totals["vFrete"])
	vNF.Add(vNF, totals["vSeg"])
	vNF.Add(vNF, totals["vOutro"])
	vNF.Add(vNF, totals["vIPI"])
	vNF.Add(vNF, totals["vST"])
	totals["vNF"].Set(vNF)

	values := map[string]any{}
	for k, v := range totals {
		values[k] = v.FloatString(2)
	}
	add("ICMSTot", values)
	if hasIS {
		add("ISTot", map[string]any{"vIS": vISTot.FloatString(2)})
	}
	if hasIBSCBS {
		add("IBSCBSTot", map[string]any{
			"vBCIBSCBS":             ibsTot["vBCIBSCBS"].FloatString(2),
			"gIBS_vIBS":             ibsTot["vIBS"].FloatString(2),
			"gIBS_vCredPres":        "0.00",
			"gIBS_vCredPresCondSus": "0.00",
			"gIBSUF_vDif":           "0.00",
			"gIBSUF_vDevTrib":       "0.00",
			"gIBSUF_vIBSUF":         ibsTot["vIBSUF"].FloatString(2),
			"gIBSMun_vDif":          "0.00",
			"gIBSMun_vDevTrib":      "0.00",
			"gIBSMun_vIBSMun":       ibsTot["vIBSMun"].FloatString(2),
			"gCBS_vDif":             "0.00",
			"gCBS_vDevTrib":         "0.00",
			"gCBS_vCBS":             ibsTot["vCBS"].FloatString(2),
			"gCBS_vCredPres":        "0.00",
			"gCBS_vCredPresCondSus": "0.00",
		})
	}
	add("transp", map[string]any{"modFrete": in.FreightMode})
	add("pag", map[string]any{})
	for _, p := range in.Payments {
		add("detPag", map[string]any{"indPag": 0, "tPag": p.Method, "vPag": p.Amount})
	}
	if in.AdditionalInfo != "" {
		add("infAdic", map[string]any{"infCpl": in.AdditionalInfo})
	}
	return tags
}

func coalesceStr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func (in Invoice) ValidateSupportedTaxes() error {
	for _, i := range in.Items {
		t := i.Taxes.ICMS
		if t.Origin < 0 || t.Origin > 8 {
			return errors.New("origem ICMS inválida")
		}
		if t.CSOSN != "" && t.CST != "" {
			return errors.New("campos ICMS incompatíveis: informe apenas CST ou CSOSN")
		}
		if t.CSOSN != "" {
			switch t.CSOSN {
			case "102", "103", "300", "400":
				if t.Base != "" || t.Rate != "" || t.Amount != "" {
					return errors.New("campos ICMS incompatíveis com o CSOSN suportado")
				}
			case "101":
				if t.CreditRate == "" || t.CreditAmount == "" {
					return errors.New("CSOSN 101 exige credit_rate e credit_amount")
				}
			case "201", "202", "203":
				if t.BaseST == "" || t.RateST == "" || t.AmountST == "" {
					return fmt.Errorf("CSOSN %s exige base_st, rate_st e amount_st", t.CSOSN)
				}
				if t.CSOSN == "201" && (t.CreditRate == "" || t.CreditAmount == "") {
					return errors.New("CSOSN 201 exige credit_rate e credit_amount")
				}
			case "500", "900":
				// Optional retained ST or custom fields
			default:
				return fmt.Errorf("CSOSN %s ainda não mapeado", t.CSOSN)
			}
		} else {
			switch t.CST {
			case "00":
				if t.Base == "" || t.Rate == "" || t.Amount == "" {
					return errors.New("ICMS CST 00 exige base, alíquota e valor")
				}
			case "10":
				if t.Base == "" || t.Rate == "" || t.Amount == "" || t.BaseST == "" || t.RateST == "" || t.AmountST == "" {
					return errors.New("ICMS CST 10 exige base, rate, amount, base_st, rate_st e amount_st")
				}
			case "20":
				if t.ReductionRate == "" || t.Base == "" || t.Rate == "" || t.Amount == "" {
					return errors.New("ICMS CST 20 exige reduction_rate, base, rate e amount")
				}
			case "30":
				if t.BaseST == "" || t.RateST == "" || t.AmountST == "" {
					return errors.New("ICMS CST 30 exige base_st, rate_st e amount_st")
				}
			case "40", "41", "50", "51", "60", "90":
				// Valid non-taxed, deferred, ST retained or generic CSTs
			case "70":
				if t.ReductionRate == "" || t.Base == "" || t.Rate == "" || t.Amount == "" || t.BaseST == "" || t.RateST == "" || t.AmountST == "" {
					return errors.New("ICMS CST 70 exige reduction_rate, base, rate, amount, base_st, rate_st e amount_st")
				}
			default:
				return errors.New("ICMS suportado: CST 00/10/20/30/40/41/50/51/60/70/90 ou CSOSN 101/102/103/201/202/203/300/400/500/900")
			}
		}

		if i.Taxes.ICMSUFDest != nil {
			if i.Taxes.ICMSUFDest.InternalRate == "" || i.Taxes.ICMSUFDest.InterstateRate == "" {
				return errors.New("DIFAL (icms_uf_dest) exige internal_rate e interstate_rate")
			}
		}
		if i.Taxes.IBSCBS != nil {
			if i.Taxes.IBSCBS.CST == "" || i.Taxes.IBSCBS.ClassTrib == "" {
				return errors.New("IBS/CBS exige cst e c_class_trib")
			}
		}
		if i.Taxes.IS != nil {
			if i.Taxes.IS.CST == "" || i.Taxes.IS.ClassTrib == "" || i.Taxes.IS.Base == "" || i.Taxes.IS.Rate == "" {
				return errors.New("Imposto Seletivo (is) exige cst, c_class_trib, base e rate")
			}
		}

		for _, v := range []Tax{i.Taxes.PIS, i.Taxes.COFINS} {
			if v.CSOSN != "" || v.Origin != 0 {
				return errors.New("CSOSN/origem não se aplicam a PIS/COFINS neste contrato")
			}

			switch v.CST {
			case "01", "02":
				if v.Base == "" || v.Rate == "" || v.Amount == "" {
					return errors.New("PIS/COFINS exige base, alíquota e valor")
				}
			case "04", "05", "06", "07", "08", "09", "49":
				for _, field := range []string{v.Base, v.Rate, v.Amount} {
					if field != "" {
						n, e := amount(field)
						if e != nil || n.Sign() != 0 {
							return errors.New("base, alíquota e valor não podem ser ignorados para CST PIS/COFINS não tributado")
						}
					}
				}
			case "50", "70", "71", "72", "73", "74", "75", "98", "99":
				// Supported extended PIS/COFINS CSTs
			default:
				return errors.New("CST PIS/COFINS ainda não mapeado")
			}
		}
	}
	return nil
}
