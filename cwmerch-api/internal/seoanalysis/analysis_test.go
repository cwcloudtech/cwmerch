package seoanalysis

import (
	"strings"
	"testing"
)

func find(r Report, id string) *Assessment {
	for i := range r.Assessments {
		if r.Assessments[i].ID == id {
			return &r.Assessments[i]
		}
	}
	return nil
}

func TestKeyphraseMatching(t *testing.T) {
	kp := NewKeyphrase("chaise en bois", "fr")
	if !kp.In("Nos chaises en Bois massif") {
		t.Fatal("plural and case should match")
	}
	if !kp.In("Du bois pour une chaise") {
		t.Fatal("any order should match")
	}
	if kp.In("Une table en bois") {
		t.Fatal("missing content word must not match")
	}
	exact := NewKeyphrase(`"chaise en bois"`, "fr")
	if exact.In("bois de chaise") || !exact.In("une chaise en bois") {
		t.Fatal("quoted keyphrase is the exact phrase")
	}
	if !kp.StartsWith("Chaise en bois – Ma boutique") || kp.StartsWith("Ma boutique – chaise en bois") {
		t.Fatal("StartsWith")
	}
	if !NewKeyphrase("été", "fr").In("Soldes d'ete") {
		t.Fatal("accents are folded")
	}
}

func TestAnalyzeProduct(t *testing.T) {
	text := "<p>Cette chaise en bois est fabriquée à la main. " + strings.Repeat("Le confort est au rendez-vous avec une assise solide. ", 30) +
		"</p><h2>Pourquoi une chaise en bois ?</h2><p>Voir notre <a href=\"/boutique\">boutique</a> et <a href=\"https://fr.wikipedia.org/wiki/Chaise\">Wikipédia</a>.</p>"
	res := Analyze(Input{Kind: KindProduct, Locale: "fr", Keyphrase: "chaise en bois",
		Title: "Chaise en bois artisanale – Ma boutique de meubles en ligne", Description: "Découvrez notre chaise en bois artisanale, fabriquée à la main en France avec des essences durables. Livraison offerte dès 50 € d'achat.",
		DescriptionSet: true, Slug: "chaise-en-bois", Name: "Chaise en bois", Text: text,
		ShortDescription: "Une chaise en bois massif, solide et confortable, fabriquée à la main dans notre atelier avec du chêne français issu de forêts gérées durablement.",
		Images:           []Image{{Src: "a.jpg", Alt: "Chaise en bois"}}, SKU: "CH-1", SiteURL: "https://shop.example"})
	for id, want := range map[string]string{"keyphraseLength": Good, "keyphraseIntroduction": Good, "titleKeyphrase": Good,
		"slugKeyphrase": Good, "metaDescriptionKeyphrase": Good, "metaDescriptionLength": Good, "internalLinks": Good,
		"outboundLinks": Good, "images": Good, "imageKeyphrase": Good, "textLength": Good, "sku": Good, "productIdentifiers": OK,
		"shortDescription": Good, "previouslyUsedKeyphrase": Good} {
		a := find(res.SEO, id)
		if a == nil || a.Rating != want {
			t.Errorf("%s = %+v, want %s", id, a, want)
		}
	}
	if res.SEO.Rating != Good {
		t.Errorf("seo rating %s (%d)", res.SEO.Rating, res.SEO.Score)
	}
	if a := find(res.Readability, "consecutiveSentences"); a == nil || a.Rating == Good {
		t.Errorf("repeated sentences should be flagged: %+v", a)
	}
	if !strings.Contains(find(res.SEO, "keyphraseLength").Title, "requête") {
		t.Error("messages should be in French")
	}
}

func TestNoKeyphraseAndEmptyMeta(t *testing.T) {
	res := Analyze(Input{Kind: KindProduct, Locale: "en", Title: "", Text: "<p>Short.</p>"})
	if a := find(res.SEO, "keyphraseLength"); a == nil || a.Rating != Bad {
		t.Fatalf("keyphraseLength %+v", a)
	}
	for _, id := range []string{"metaDescriptionLength", "titleWidth", "images", "shortDescription"} {
		if a := find(res.SEO, id); a == nil || a.Rating != Bad {
			t.Errorf("%s %+v", id, a)
		}
	}
	if a := find(res.Readability, "textPresence"); a == nil {
		t.Error("textPresence expected for a short text")
	}
	if res.SEO.Assessments[0].Rating != Bad {
		t.Error("problems first")
	}
}

func TestReadabilityEnglish(t *testing.T) {
	long := strings.Repeat("The chair was made by hand in our workshop and it is sold with a warranty of five years. ", 12)
	easy := strings.Repeat("However, this chair is nice. It is solid. You will love it. ", 20)
	r1 := Analyze(Input{Kind: KindProduct, Locale: "en", Text: "<p>" + long + "</p>"})
	if a := find(r1.Readability, "passiveVoice"); a == nil || a.Rating == Good {
		t.Errorf("passive voice should be flagged: %+v", a)
	}
	r2 := Analyze(Input{Kind: KindProduct, Locale: "en", Text: "<p>" + easy + "</p>"})
	if r2.Flesch < 70 || r2.Flesch <= r1.Flesch {
		t.Errorf("flesch easy=%v hard=%v", r2.Flesch, r1.Flesch)
	}
	if a := find(r2.Readability, "transitionWords"); a == nil || a.Rating != Good {
		t.Errorf("transition words: %+v", a)
	}
}

func TestTitleWidth(t *testing.T) {
	if w := TitleWidth(strings.Repeat("a", 60)); w < 550 || w > 700 {
		t.Errorf("width %d", w)
	}
}

func TestNoKeyphraseNoScore(t *testing.T) {
	res := Analyze(Input{Kind: KindProduct, Locale: "en", Title: "A chair", Text: "<p>A chair.</p>"})
	if res.SEO.Score != -1 || res.SEO.Rating != NA {
		t.Fatalf("score %d rating %s", res.SEO.Score, res.SEO.Rating)
	}
	home := Analyze(Input{Kind: KindHome, Locale: "en", Title: "My shop – the best chairs in town", Description: strings.Repeat("x", 130), DescriptionSet: true})
	if home.SEO.Rating == NA {
		t.Fatal("home pages are scored without a keyphrase")
	}
}
