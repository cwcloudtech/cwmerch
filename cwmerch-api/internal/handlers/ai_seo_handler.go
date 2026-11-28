package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"cwmerch-api/internal/ai"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/seoanalysis"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/utils"
)

// WithSEO wires the SEO service behind the SEO suggestions.
func (h *AIHandler) WithSEO(svc *SEOService) *AIHandler {
	h.seo = svc
	return h
}

type seoSuggestPayload struct {
	seoTarget
	ConnectionID string `json:"connectionId"`
}

// SEOSuggestion is one change the model proposes; Field names what it
// applies to (empty Value: advice only).
type SEOSuggestion struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type seoSuggestResponse struct {
	Summary     string          `json:"summary"`
	Suggestions []SEOSuggestion `json:"suggestions"`
	Page        seoPage         `json:"page"`
	Model       string          `json:"model"`
}

var seoSuggestFields = map[string]bool{"focusKeyphrase": true, "title": true, "description": true, "slug": true,
	"shortDescription": true, "text": true, "ogTitle": true, "ogDescription": true, "breadcrumbTitle": true, "": true}

const seoSuggestSystem = `You are an SEO expert for an online shop, working like the best WordPress SEO assistants.
You receive a storefront page (a product, a product category or tag, the home page or the shop page): its name, its text,
its current SEO data, its SEO title and meta description as Google shows them, and the results of an SEO and
readability analysis (the problems and improvements). Propose concrete, ready-to-use improvements:
- focusKeyphrase: 1 to 4 words people really search for, specific to this page (not already used elsewhere when told so)
- title: the SEO title, starting with the focus keyphrase, at most about 60 characters; you may use %%sitename%% and %%sep%%
- description: the meta description, 120 to 156 characters, containing the focus keyphrase once, with a call to action
- slug: lowercase-with-dashes, short, containing the keyphrase (only when the current one is clearly bad)
- shortDescription (products): 20 to 50 words, HTML paragraphs allowed
- text: the full rewritten HTML text ONLY when the text is missing, too short or has readability problems; keep every fact,
  never invent features, prices, materials, sizes or claims; use <h2> subheadings, short paragraphs, transition words,
  the focus keyphrase in the first sentence
- ogTitle / ogDescription: only when social sharing would benefit from different wording
- advice without a value (field ""): anything else (images, alt texts, links, SKU/GTIN...)
Answer with ONE JSON object and nothing else:
{"summary": "2-3 sentences on the page's SEO state", "suggestions": [{"field": "...", "value": "...", "reason": "why, briefly"}]}
Never invent facts about the product. Keep the page's language.`

var jsonFence = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*(.*?)\\s*```\\s*$")

// parseSEOSuggestions reads the model's JSON (tolerating code fences or text
// around it).
func parseSEOSuggestions(text string) (string, []SEOSuggestion, bool) {
	t := strings.TrimSpace(text)
	if m := jsonFence.FindStringSubmatch(t); m != nil {
		t = m[1]
	}
	if i, j := strings.Index(t, "{"), strings.LastIndex(t, "}"); i >= 0 && j > i {
		t = t[i : j+1]
	}
	var out struct {
		Summary     string          `json:"summary"`
		Suggestions []SEOSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return strings.TrimSpace(text), []SEOSuggestion{}, false
	}
	kept := []SEOSuggestion{}
	for _, s := range out.Suggestions {
		s.Field = strings.TrimSpace(s.Field)
		if !seoSuggestFields[s.Field] {
			s.Field = ""
		}
		if s.Field != "" && strings.TrimSpace(s.Value) == "" {
			s.Field = ""
		}
		kept = append(kept, s)
	}
	return out.Summary, kept, true
}

func seoPrompt(p seoPage, in *seoanalysis.Input) string {
	type issue struct {
		Check  string `json:"check"`
		Rating string `json:"rating"`
		Text   string `json:"text"`
	}
	issues := func(r seoanalysis.Report) []issue {
		out := []issue{}
		for _, a := range r.Assessments {
			if a.Rating != seoanalysis.Good {
				out = append(out, issue{a.Title, a.Rating, a.Text})
			}
		}
		return out
	}
	text := in.Text
	if r := []rune(text); len(r) > 8000 {
		text = string(r[:8000]) + "…"
	}
	data := map[string]any{
		"kind": p.Kind, "name": p.Name, "path": p.Path, "slug": p.Slug, "contentLanguage": in.Locale,
		"focusKeyphrase": p.Meta.FocusKeyphrase, "keyphraseAlsoUsedBy": in.KeyphraseUsedBy,
		"seoTitleTemplate": p.Meta.Title, "metaDescriptionTemplate": p.Meta.Description,
		"seoTitleAsShown": p.Head.Title, "metaDescriptionAsShown": p.Head.Description, "metaDescriptionIsAutomatic": p.Head.DescriptionIsAuto,
		"text": text, "shortDescription": in.ShortDescription, "sku": in.SKU, "imageAlts": in.Images,
		"wordCount": p.Analysis.WordCount, "seoScore": p.Analysis.SEO.Score, "readabilityScore": p.Analysis.Readability.Score,
		"seoProblems": issues(p.Analysis.SEO), "readabilityProblems": issues(p.Analysis.Readability),
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	return "Analyse this page and propose improvements:\n" + string(raw)
}

// SEOSuggest is POST /v1/admin/seo/suggest: the analysis of a page, then the
// AI's suggestions to improve it (the "Analyse this page" button), spent on
// the caller's AI subscription.
func (h *AIHandler) SEOSuggest(w http.ResponseWriter, r *http.Request) {
	userID, ok := aiUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", CodeInvalidCredentials)
		return
	}
	if h.seo == nil {
		writeError(w, http.StatusServiceUnavailable, "SEO is not available", CodeNotFound)
		return
	}
	var p seoSuggestPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	page, err := h.seo.resolve(r.Context(), p.seoTarget, lngOf(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	connection, err := h.usable(r, userID, p.ConnectionID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound) && strings.TrimSpace(p.ConnectionID) != "":
			writeError(w, http.StatusNotFound, "Connection not found", CodeNotFound)
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusPreconditionRequired, "Add an AI subscription first", codeAINotConfigured)
		default:
			writeStoreError(w, err)
		}
		return
	}
	locale := normalizeLocale(utils.If(page.Input.MessageLocale != "", page.Input.MessageLocale, page.Input.Locale))
	answer, err := h.client.Ask(r.Context(), connection, ai.Request{
		System: seoSuggestSystem + "\n\nWrite the summary and the reasons in " + languageName(locale) +
			". Write the suggested values (titles, descriptions, keyphrases, texts) in the language of the page's own text " +
			"(the shop's language is " + languageName(normalizeLocale(page.Input.Locale)) + ").",
		Messages:  []ai.Turn{ai.TurnOf(models.AIChatRoleUser, seoPrompt(page, page.Input))},
		MaxTokens: 4096,
	})
	if err != nil {
		h.writeProviderError(w, connection, err)
		return
	}
	summary, suggestions, _ := parseSEOSuggestions(answer.Message)
	model := answeredWith(answer.Model, connection.Model)
	h.record(r, models.AuditAICall, models.AuditSubjectAIConnection, connection.ID, map[string]any{
		"provider": connection.Provider, "model": model, "input": answer.Tokens.Input, "output": answer.Tokens.Output,
		"cached": answer.Tokens.Cached, "connection": connection.Name, "feature": "seo.suggest",
	})
	notifyDiscord(h.disc, fmt.Sprintf("[AI] :robot: %s asked for SEO suggestions on %s %q (%s / %s) — tokens consumed: %d input + %d output (%d cached)",
		actingEmail(r), page.Kind, page.Name, connection.Provider, model, answer.Tokens.Input, answer.Tokens.Output, answer.Tokens.Cached))
	writeJSON(w, http.StatusOK, seoSuggestResponse{Summary: summary, Suggestions: suggestions, Page: page, Model: model})
}
