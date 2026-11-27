package handlers

import (
	"net/http"
	"strconv"
)

const (
	defaultAdminPageSize = 20
	maxAdminPageSize     = 200
)

// pagedParams reads the paging query params shared by the superuser admin
// list endpoints, matching uprodit's SearchResult convention: startIndex is a
// 0-based PAGE NUMBER (not a row offset) and maxResults is the page size.
// Absent or invalid values fall back to page 0 and the default page size, and
// maxResults is clamped to maxAdminPageSize. The row offset is startIndex *
// maxResults.
func pagedParams(r *http.Request) (startIndex, maxResults int) {
	q := r.URL.Query()
	startIndex, _ = strconv.Atoi(q.Get("startIndex"))
	// The back office pages with ?page (1-based) & ?perPage like the other
	// admin lists; accept both shapes.
	if q.Get("startIndex") == "" {
		if page, err := strconv.Atoi(q.Get("page")); err == nil && page > 0 {
			startIndex = page - 1
		}
	}
	if startIndex < 0 {
		startIndex = 0
	}
	maxResults, _ = strconv.Atoi(q.Get("maxResults"))
	if q.Get("maxResults") == "" {
		maxResults, _ = strconv.Atoi(q.Get("perPage"))
	}
	if maxResults < 1 {
		maxResults = defaultAdminPageSize
	}
	if maxResults > maxAdminPageSize {
		maxResults = maxAdminPageSize
	}
	return startIndex, maxResults
}

// pagedResponse is the list envelope the admin list endpoints return, matching
// the SearchResult shape uprodit uses and the frontend's infinite scroll and
// the assistant both expect: startIndex (0-based page), maxResults (page size),
// totalResults (the full count behind the filter) and results (this page).
type pagedResponse struct {
	StartIndex   int   `json:"startIndex"`
	MaxResults   int   `json:"maxResults"`
	TotalResults int64 `json:"totalResults"`
	Results      any   `json:"results"`
}
