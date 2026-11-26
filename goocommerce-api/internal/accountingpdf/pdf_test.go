package accountingpdf

import "testing"

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		v    float64
		dec  int
		want string
	}{
		{1234.56, 2, "1 234,56"},
		{0, 2, "0,00"},
		{-1234567.891, 2, "-1 234 567,89"},
		{999.999, 2, "1 000,00"},
		{-0.001, 2, "0,00"},
		{123, 0, "123"},
		{1234, 3, "1 234,000"},
	}
	for _, c := range cases {
		if got := FormatNumber(c.v, c.dec); got != c.want {
			t.Errorf("FormatNumber(%v, %d) = %q, want %q", c.v, c.dec, got, c.want)
		}
	}
	if got := FormatAmount(1234.5, 2, "€"); got != "1 234,50 €" {
		t.Errorf("FormatAmount = %q", got)
	}
}
