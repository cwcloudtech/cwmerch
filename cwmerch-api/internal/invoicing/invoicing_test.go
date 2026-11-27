package invoicing

import (
	"bytes"
	"strings"
	"testing"

	"cwmerch-api/internal/mdpdf"
)

func sampleModel(mode string) Model {
	set := Settings{TaxMode: mode, VatLabel: "TVA", StampLabel: "Timbre fiscal", Locale: "fr",
		VatExemptionMention: "TVA non applicable, art. 293 B du CGI", PaymentTerms: "À réception",
		Seller: Seller{Name: "Fleurs SARL", Address: "1 rue des Roses", Postcode: "1000", City: "Tunis", Country: "TN",
			RegistrationLabel: "Matricule fiscal", RegistrationNumber: "1234567/A"}}
	return Model{Settings: set, HasVAT: HasVAT(mode), HasStamp: HasStamp(mode), T: labels["fr"],
		Invoice: Header{Number: "FAC-2026-00001", IssuedAt: "27/11/2026", OrderNumber: "42", OrderDate: "26/11/2026", PaymentMethod: "Konnect", Currency: "TND"},
		Seller:  SellerView{Seller: set.Seller, CountryName: "Tunisie"},
		Buyer:   Buyer{Name: "Amel Ben Ali", AddressLines: []string{"5 avenue Bourguiba", "1001 Tunis"}, Email: "amel@example.tn"},
		Lines:   []Line{{Name: "Bouquet", Quantity: "2", UnitHT: "25,000 DT", VatRate: "19 %", TotalHT: "50,000 DT"}},
		Taxes:   []Tax{{Label: "TVA", Rate: "19 %", Base: "50,000 DT", Amount: "9,500 DT"}},
		Totals:  Totals{HT: "50,000 DT", Tax: "9,500 DT", Stamp: "1,000 DT", TTC: "60,500 DT"}}
}

func TestDefaultTemplateVATAndStamp(t *testing.T) {
	md, err := RenderMarkdown("", sampleModel(TaxVATStamp))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Facture FAC-2026-00001", "Matricule fiscal : 1234567/A", "| TVA 19 % (sur 50,000 DT) | 9,500 DT |",
		"| Timbre fiscal | 1,000 DT |", "**60,500 DT**", "| Désignation | Qté | P.U. HT | TVA | Total HT |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "293 B") {
		t.Error("VAT exemption mention shown on a VAT invoice")
	}
	pdf, err := mdpdf.Render(md, mdpdf.Options{Footer: "Fleurs SARL"})
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("pdf: %v", err)
	}
}

func TestDefaultTemplateNoVAT(t *testing.T) {
	md, err := RenderMarkdown("", sampleModel(TaxNone))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "TVA non applicable, art. 293 B du CGI") || strings.Contains(md, "Timbre") || strings.Contains(md, "| TVA |") {
		t.Errorf("no-VAT invoice wrong:\n%s", md)
	}
}

func TestCustomTemplateErrors(t *testing.T) {
	if _, err := RenderMarkdown("{{.Invoice.Number", sampleModel(TaxVAT)); err == nil {
		t.Fatal("a broken template must be refused")
	}
	md, err := RenderMarkdown("Invoice {{.Invoice.Number}} for {{upper .Buyer.Name}}", sampleModel(TaxVAT))
	if err != nil || md != "Invoice FAC-2026-00001 for AMEL BEN ALI" {
		t.Fatalf("custom: %v %q", err, md)
	}
}
