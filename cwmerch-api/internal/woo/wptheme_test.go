package woo

import "testing"

func TestNormColor(t *testing.T) {
	cases := map[string]string{
		"#E384AA":            "#e384aa",
		"#fff":               "#ffffff",
		"#2ea3f280":          "#96d1f8",
		"rgba(0,0,0,0.6)":    "#666666",
		"rgb(10, 20, 30)":    "#0a141e",
		"rgba(0, 0, 0, 50%)": "#808080",
		"":                   "",
		"none":               "",
		"var(--x)":           "",
		"#12":                "",
	}
	for in, want := range cases {
		if got := normColor(in); got != want {
			t.Errorf("normColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadableOn(t *testing.T) {
	for bg, want := range map[string]string{"#ffffff": "#0f172a", "#0a7fae": "#ffffff", "#222222": "#ffffff", "#e384aa": "#0f172a", "#f8f8f8": "#0f172a"} {
		if got := readableOn(bg); got != want {
			t.Errorf("readableOn(%q) = %q, want %q", bg, got, want)
		}
	}
}

func TestMergeColors(t *testing.T) {
	base := Themes[0].Colors
	got := mergeColors(base, Colors{Primary: "#e384aa", Footer: "#222222"})
	if got.Primary != "#e384aa" || got.Accent != "#e384aa" || got.PrimaryText != "#0f172a" || got.FooterText != "#ffffff" || got.Background != base.Background {
		t.Fatalf("unexpected merge: %+v", got)
	}
}
