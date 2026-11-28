package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"cwmerch-api/internal/ai"
)

// The assistant's tool (ai-instruct-132). The model is handed cwmerch's own API and
// expected to call it — read the user's data, and generate reports and invoices —
// before it answers, rather than telling the user how to click through the UI. One
// tool: cwmerch_api, a GET/POST against this very API as the signed-in user (its own
// Auth / OrgMembership / RequireRole gates decide what comes back).
const (
	toolCwclockAPIName = "cwmerch_api"
	// toolSEOName runs the SEO and readability analysis of a page.
	toolSEOName = "cwmerch_seo_analyze"

	// apiToolTimeout bounds one cwmerch_api call. Generous: a report over a long
	// range, or an invoice render, takes a moment.
	apiToolTimeout = 30 * time.Second
	// apiToolMaxBytes caps how much of a response is read into memory.
	apiToolMaxBytes = 2 << 20
	// apiToolTextCap caps the text body handed back to the model, so a large report
	// grounds an answer without blowing the prompt budget.
	apiToolTextCap = 64 * 1024
)

// apiToolClient is the HTTP client the cwmerch_api tool calls this very API with.
// Its proxy is pinned off: the call is to the loopback self-base, and routing a
// self-call through a corporate proxy would hang, then fail.
var apiToolClient = &http.Client{
	Timeout:   apiToolTimeout,
	Transport: &http.Transport{Proxy: nil},
}

// aiTools is the tool catalogue offered to the model each chat turn.
func (h *AIHandler) aiTools() []ai.Tool {
	return []ai.Tool{
		{
			Name: toolCwclockAPIName,
			Description: "Call cwmerch's own HTTP API (the shop: products, orders, customers, coupons, " +
				"reports, accounting exports, settings) AS THE SIGNED-IN USER, to ground every answer in real data. " +
				"It runs with the user's OWN permissions: a customer only reaches /v1/shop/* and their own " +
				"/v1/users/me/*; shop managers reach /v1/admin/*. A forbidden result means the user is not allowed: " +
				"say so and do not retry. Use GET to read. POST/PUT/DELETE change the shop (create a product, " +
				"change an order status, refund, add a note, edit a coupon...): ONLY after you described the exact " +
				"change and the user explicitly agreed, and then pass confirmed=true — a write without it is refused. " +
				"Read-only POSTs (pricing a cart, testing a payment provider, testing a database connection for the " +
				"clone) need no confirmation. Cloning data between databases (MySQL/MariaDB <-> PostgreSQL, export or " +
				"import, administrators only) always starts with POST /v1/admin/clone/test, then the user's explicit yes, " +
				"then POST /v1/admin/clone with confirm=true, then GET /v1/admin/clone/job until it is done. " +
				"A PDF export (accounting turnover / balance / grand livre with ?format=pdf) comes back as a " +
				"temporary download link to give the user verbatim. The skill lists the useful paths.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"method": map[string]any{
						"type":        "string",
						"enum":        []string{"GET", "POST", "PUT", "DELETE"},
						"description": "GET to read (default). POST/PUT/DELETE to change data, only once the user confirmed.",
					},
					"path": map[string]any{
						"type": "string",
						"description": "The API path, starting with '/v1/', with its own query, e.g. " +
							"'/v1/admin/orders/?status=processing', '/v1/admin/reports/summary?from=2026-01-01&to=2026-01-31', " +
							"'/v1/admin/accounting/ledger?year=2026&format=pdf'.",
					},
					"body": map[string]any{
						"type":        "object",
						"description": "JSON body for POST/PUT. Omit for GET/DELETE.",
					},
					"confirmed": map[string]any{
						"type":        "boolean",
						"description": "true only when the user explicitly approved this exact write in the conversation.",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name: toolSEOName,
			Description: "Run the SEO analysis (the classic WordPress SEO checks: focus keyphrase in title, introduction, meta description, slug, " +
				"subheadings and image alts, keyphrase density, title width, meta description length, text length, links, images, " +
				"product short description / SKU / GTIN) and the readability analysis (sentence and paragraph length, subheadings, " +
				"transition words, passive voice, Flesch reading ease) of a storefront page, as the signed-in shop manager. " +
				"Returns the page's SEO data (focus keyphrase, SEO title and meta description templates, robots, social), the resolved " +
				"head (title, description as Google shows them), the template variables and every assessment with its rating " +
				"(good / ok / bad). Use it to audit a product, a category, a tag, the home page or the shop page, then suggest concrete " +
				"improvements (an SEO title under ~60 characters starting with the keyphrase, a 120-156 character meta description " +
				"containing it, a better focus keyphrase, text changes...). To apply them, write with cwmerch_api PUT " +
				"/v1/admin/seo/products/{id} or /v1/admin/seo/terms/{product_cat|product_tag}/{id} (the full SEO object) or PUT " +
				"/v1/admin/products/{id} for the product's text, only after the user agreed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": []string{"product", "category", "tag", "home", "shop"},
						"description": "What to analyse (with id for product/category/tag). Omit when giving a path."},
					"id":   map[string]any{"type": "integer", "description": "The product's or term's id."},
					"path": map[string]any{"type": "string", "description": "Or the storefront path, e.g. '/product/oak-chair/'."},
				},
			},
		},
	}
}

// runTool executes one tool call and returns its result as a JSON string. It never
// returns an error: a failed tool reports the failure to the model as data, so the
// agent loop and the chat carry on.
func (h *AIHandler) runTool(r *http.Request, call ai.ToolCall) string {
	switch call.Name {
	case toolCwclockAPIName:
		var args struct {
			Method    string          `json:"method"`
			Path      string          `json:"path"`
			Body      json.RawMessage `json:"body"`
			Confirmed bool            `json:"confirmed"`
		}
		_ = json.Unmarshal(call.Args, &args)
		m := strings.ToUpper(strings.TrimSpace(args.Method))
		if m != "" && m != http.MethodGet && !args.Confirmed && !(m == http.MethodPost && readOnlyPost(args.Path)) {
			return aiToolJSON(map[string]any{"error": "write refused: describe the exact change to the user, ask for their explicit " +
				"agreement, and only then call again with confirmed=true"})
		}
		return h.toolCwclockAPI(r, args.Method, args.Path, args.Body)
	case toolSEOName:
		var args struct {
			Kind string `json:"kind"`
			ID   uint64 `json:"id"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(call.Args, &args)
		body, _ := json.Marshal(map[string]any{"kind": args.Kind, "id": args.ID, "path": args.Path})
		return h.toolCwclockAPI(r, http.MethodPost, "/v1/admin/seo/analyze", body)
	default:
		return aiToolErrorJSON("unknown tool")
	}
}

// toolCwclockAPI performs one GET or POST against cwmerch's own API as the signed-in
// user and returns the response as a JSON string the model reads. The whole point is
// that it carries the CALLER's own credential — the X-Api-Key or Bearer token that
// authenticated the incoming chat — so the API's own authorization decides what
// comes back: an admin reaches invoice generation, a member is refused (403),
// surfaced here as data so the model says "you don't have permission". GET and POST
// only (no delete/update), so the model can read and generate but never destroy. The
// path is pinned to this API (relative, under /v1/), so the user's token can never
// be pointed somewhere else. Every failure is returned as JSON, not an error, so a
// bad tool call never breaks the chat.
func (h *AIHandler) toolCwclockAPI(r *http.Request, method, path string, body json.RawMessage) string {
	if h.apiBaseURL == "" {
		return aiToolJSON(map[string]any{"error": "cwmerch api access unavailable"})
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return aiToolJSON(map[string]any{"error": "method must be GET, POST, PUT or DELETE"})
	}

	// The model supplies a server-relative path under /v1/ only. Reject anything that
	// could reach off this API — an absolute URL, a scheme, a protocol-relative
	// //host, or an opaque/host component.
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return aiToolJSON(map[string]any{"error": "path must be an absolute path on this API, e.g. /v1/admin/orders/"})
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return aiToolJSON(map[string]any{"error": "path must be a relative API path, not a full URL"})
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/v1/") {
		return aiToolJSON(map[string]any{"error": "path must start with /v1/"})
	}

	requestURI := parsed.EscapedPath()
	if parsed.RawQuery != "" {
		requestURI += "?" + parsed.RawQuery
	}
	endpoint := strings.TrimRight(h.apiBaseURL, "/") + requestURI

	var reqBody io.Reader
	if method == http.MethodPost || method == http.MethodPut {
		if len(bytes.TrimSpace(body)) == 0 || !json.Valid(body) {
			body = json.RawMessage("{}")
		}
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(r.Context(), method, endpoint, reqBody)
	if err != nil {
		return aiToolJSON(map[string]any{"error": "could not build the request"})
	}
	// Forward whatever credential authenticated the chat (X-Api-Key for mobile,
	// Bearer for web); the self-call then acts as exactly this user.
	if apiKey := r.Header.Get("X-Api-Key"); apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cid := r.Header.Get("X-Cid"); cid != "" {
		req.Header.Set("X-Cid", cid)
	}
	req.Header.Set("Accept", "application/json, application/pdf, text/csv, text/plain")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := apiToolClient.Do(req)
	if err != nil {
		return aiToolJSON(map[string]any{"error": "the cwmerch API could not be reached"})
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return aiToolJSON(map[string]any{
			"status": resp.StatusCode, "forbidden": true,
			"note": "the signed-in user is not allowed to do this; tell them they do not have " +
				"permission for it and do not retry",
		})
	}

	// A JSON/text body is read small — just enough to ground an answer. A generated
	// file (PDF/CSV) is binary and must be read whole so it can be parked for the
	// user to download, hence the larger cap when a download store is wired.
	binary := !aiLooksTextual(contentType)
	limit := int64(apiToolMaxBytes)
	if binary && h.downloads != nil && h.downloads.MaxBytes() > 0 {
		limit = h.downloads.MaxBytes()
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, limit))

	if binary || !utf8.Valid(raw) {
		return h.toolFileResult(r, method, resp.StatusCode, contentType, resp.Header.Get("Content-Disposition"), raw)
	}

	text, truncated := aiCapText(raw)
	return aiToolJSON(map[string]any{
		"status": resp.StatusCode, "contentType": contentType, "body": text, "truncated": truncated,
	})
}

// toolFileResult handles a binary (file) response — an invoice or an exported
// report PDF/CSV. When a download store is wired and the caller is known, it parks
// the bytes and hands the model a `downloadUrl` to post as a clickable link;
// otherwise it just names the file. Either way the model never sees the bytes.
func (h *AIHandler) toolFileResult(r *http.Request, method string, status int, contentType, disposition string, raw []byte) string {
	filename := aiDispositionFilename(disposition)
	result := map[string]any{"status": status, "contentType": contentType}
	if filename != "" {
		result["filename"] = filename
	}

	if status < 300 && len(raw) > 0 && h.downloads != nil {
		if userID, ok := aiUserID(r); ok {
			name := filename
			if name == "" {
				name = aiFallbackFilename(contentType)
			}
			if url, _, err := h.downloads.Save(userID, name, raw); err == nil {
				result["filename"] = name
				result["downloadUrl"] = url
				result["note"] = "a file was generated. Give the user this exact downloadUrl as a clickable " +
					"Markdown link — [" + name + "](" + url + ") — and tell them the link is temporary. " +
					"Never invent, alter or guess a download URL; only ever use one a tool returned."
				return aiToolJSON(result)
			}
		}
	}

	if method == http.MethodPost && status < 300 {
		result["note"] = "a file was generated; its name is the filename above " +
			"and it is available in the app. The file's contents are not shown here."
	} else {
		result["note"] = "binary response, not shown"
	}
	return aiToolJSON(result)
}

// aiLooksTextual is true for the content types whose body is worth returning
// verbatim (JSON, plain text, CSV, XML).
func aiLooksTextual(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "json") || strings.Contains(ct, "text") ||
		strings.Contains(ct, "csv") || strings.Contains(ct, "xml")
}

// aiCapText bounds one text field handed to the model, flagging when it was cut.
func aiCapText(b []byte) (string, bool) {
	if len(b) > apiToolTextCap {
		return string(b[:apiToolTextCap]), true
	}
	return string(b), false
}

// aiDispositionFilename pulls the filename out of a Content-Disposition header (e.g.
// `attachment; filename="2026-0001.pdf"`), so an invoice generation can be reported
// by its number. "" when there is none.
func aiDispositionFilename(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(params["filename"])
}

// aiFallbackFilename names a generated file when the response carried no
// Content-Disposition, from its content type.
func aiFallbackFilename(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "pdf"):
		return "document.pdf"
	case strings.Contains(ct, "csv"):
		return "export.csv"
	default:
		return "download"
	}
}

// aiToolJSON encodes a tool result; a marshalling failure still comes back as a JSON
// error the model can read.
func aiToolJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return aiToolErrorJSON("could not encode tool result")
	}
	return string(encoded)
}

// aiToolErrorJSON is a tool's failure, reported to the model as data.
func aiToolErrorJSON(message string) string {
	encoded, _ := json.Marshal(map[string]string{"error": message})
	return string(encoded)
}

// readOnlyPost tells the POSTs that change nothing (they only compute or
// check), which the assistant may run without asking for a confirmation.
func readOnlyPost(path string) bool {
	p := strings.TrimSpace(path)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/")
	switch {
	case p == "/v1/shop/cart", p == "/v1/admin/clone/test", p == "/v1/admin/seo/analyze":
		return true
	case strings.HasPrefix(p, "/v1/admin/payment-providers/") && strings.HasSuffix(p, "/test") && strings.Count(p, "/") == 5:
		return true
	}
	return false
}
