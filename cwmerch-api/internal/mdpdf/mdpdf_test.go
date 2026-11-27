package mdpdf

import (
	"bytes"
	"testing"
)

func TestRenderSubset(t *testing.T) {
	md := "# Facture FAC-2026-00001\n\n**Vendeur SARL**\n12 rue de la Paix\n75002 Paris\n\n---\n\n" +
		"| Désignation | Qté | Total HT |\n|---|---:|---:|\n| Bouquet de roses rouges, très longue désignation qui doit passer à la ligne proprement | 2 | 60,00 € |\n| **Total** | | **60,00 €** |\n\n- TVA non applicable\n"
	pdf, err := Render(md, Options{Footer: "Mentions légales", Title: "Facture"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || len(pdf) < 1000 {
		t.Fatalf("not a PDF (%d bytes)", len(pdf))
	}
}
