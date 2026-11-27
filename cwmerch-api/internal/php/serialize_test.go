package php

import "testing"

func TestRoundTripWooCommerceValues(t *testing.T) {
	cases := []string{
		`a:1:{s:8:"customer";b:1;}`,
		`a:2:{s:8:"pa_color";a:6:{s:4:"name";s:8:"pa_color";s:5:"value";s:0:"";s:8:"position";i:0;s:10:"is_visible";i:1;s:12:"is_variation";i:1;s:11:"is_taxonomy";i:1;}s:4:"size";a:6:{s:4:"name";s:4:"Size";s:5:"value";s:5:"S | M";s:8:"position";i:1;s:10:"is_visible";i:1;s:12:"is_variation";i:0;s:11:"is_taxonomy";i:0;}}`,
		`a:3:{i:0;i:12;i:1;i:15;i:2;i:3;}`,
		`a:2:{s:7:"enabled";s:3:"yes";s:5:"title";s:14:"Virement été";}`,
	}
	for _, c := range cases {
		v, err := Unmarshal(c)
		if err != nil {
			t.Fatalf("Unmarshal(%s): %v", c, err)
		}
		if got := Marshal(v); got != c {
			t.Errorf("round trip:\n got %s\nwant %s", got, c)
		}
	}
}

func TestMultibyteLengthIsBytes(t *testing.T) {
	a := UnmarshalArray(`a:1:{s:5:"title";s:6:"Café!";}`)
	if a.String("title") != "Café!" {
		t.Fatalf("got %q", a.String("title"))
	}
	if Marshal("é") != `s:2:"é";` {
		t.Fatalf("string length must count bytes: %s", Marshal("é"))
	}
}

func TestNumericStringKeysBecomeInts(t *testing.T) {
	a := NewArray().Set("5", "x")
	if Marshal(a) != `a:1:{i:5;s:1:"x";}` {
		t.Fatalf("got %s", Marshal(a))
	}
}
