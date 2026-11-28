package woo

import "testing"

func TestReplaceSEOVars(t *testing.T) {
	vars := SEOVars{"title": "Chaise", "sitename": "Shop", "sep": "-", "sitedesc": "", "page": ""}
	cases := map[string]string{
		"%%title%% %%page%% %%sep%% %%sitename%%":    "Chaise - Shop",
		"%%sitename%% %%page%% %%sep%% %%sitedesc%%": "Shop",
		"%%sitename%%":          "Shop",
		"%%unknown%% %%title%%": "Chaise",
		"Plain":                 "Plain",
	}
	for in, want := range cases {
		if got := ReplaceSEOVars(in, vars); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestHeadResolvesSocialVariables(t *testing.T) {
	h := SEOHead{Title: "Shop", Description: "d"}
	h.finish(SEOSettings{OpenGraph: true}, SEOMeta{OGTitle: "%%sitename%% %%sep%% %%sitedesc%%"}, "", SEOVars{"sitename": "Shop", "sep": "-"})
	if h.OGTitle != "Shop" || h.TwitterTitle != "Shop" {
		t.Fatalf("og %q twitter %q", h.OGTitle, h.TwitterTitle)
	}
}
