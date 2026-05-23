package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"runway/reviews/internal/store"
)

// MaxPages caps how many pages of the Apple customer-reviews RSS feed we
// walk per poll. Apple's CDN itself caps the feed at ~10 pages × ~50
// reviews each — page 11+ returns empty for most apps — so 10 is the
// practical ceiling. Exposed as a var (and overridable via the MAX_PAGES
// env var in cmd/server/main.go) so the cap can be tuned without
// recompiling if Apple changes the limit or for test fixtures.
var MaxPages = 10

// maxFeedBodyBytes caps the size of an Apple RSS response we'll consume.
// A legitimate 50-review page is ~80 KB; the cap protects against a
// compromised or misbehaving upstream streaming unbounded bytes within
// the 10s client timeout.
const maxFeedBodyBytes = 5 * 1024 * 1024

// existingSortByRegex matches any /sortBy=.../ path segment so
// CanonicalizeURL can strip it before re-inserting /sortBy=mostRecent/.
var existingSortByRegex = regexp.MustCompile(`/sortBy=[^/]+`)

// existingPageRegex matches any /page=N/ path segment. The example URL in
// the assignment includes /page=1/ — without stripping it, pageURL appends
// a second /page=N/ producing /page=1/page=2/json, which Apple tolerates
// but is undocumented behaviour we should not rely on.
var existingPageRegex = regexp.MustCompile(`/page=\d+`)

// CanonicalizeURL rewrites a user-submitted Apple RSS URL into the exact form
// the poller's pagination logic depends on: /sortBy=mostRecent/json. The
// per-page early-stop in FetchAll assumes newest-first ordering inside each
// page, which only holds for sortBy=mostRecent — rather than reject URLs
// missing the segment, we forgivingly rewrite them. Any pre-existing
// /page=N/ segment is stripped so pageURL has a clean base to append to.
func CanonicalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = existingSortByRegex.ReplaceAllString(u, "")
	u = existingPageRegex.ReplaceAllString(u, "")
	u = ensureJSON(u)
	// Insert /sortBy=mostRecent immediately before the trailing /json.
	idx := strings.LastIndex(u, "/json")
	if idx == -1 {
		return u + "/sortBy=mostRecent/json"
	}
	return u[:idx] + "/sortBy=mostRecent" + u[idx:]
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	// Disable redirect following to prevent SSRF: a crafted RSS URL could
	// trigger a redirect to an internal network resource or cloud metadata endpoint.
	// Apple's RSS feed never legitimately redirects, so any 3xx is rejected.
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type itunesLookupResponse struct {
	Results []struct {
		TrackName string `json:"trackName"`
	} `json:"results"`
}

// lookupBaseURL is the iTunes lookup endpoint prefix. Exposed as a var
// (not a const) so tests can swap it for a httptest server URL without
// fighting Go's lack of monkey-patching for hardcoded strings. In prod
// it's always the real Apple endpoint.
var lookupBaseURL = "https://itunes.apple.com/lookup?id="

// LookupAppName resolves an app name from the iTunes lookup API by appId.
// Falls back to the empty string on any error so the caller can supply a default.
func LookupAppName(ctx context.Context, appID string) string {
	url := lookupBaseURL + appID
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "RunwayReviewsBot/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	// Register the close BEFORE the status check — otherwise a non-200
	// response returns without closing the body and leaks a connection
	// from the transport's idle pool on every failed lookup.
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	// Cap the body for symmetry with FetchPage — a misbehaving upstream
	// can stream unbounded bytes within the 10 s client timeout.
	var lookup itunesLookupResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxFeedBodyBytes)).Decode(&lookup); err != nil {
		return ""
	}
	if len(lookup.Results) > 0 {
		return lookup.Results[0].TrackName
	}
	return ""
}

// rssEntry is one review (or the app-info entry) inside the Apple feed.
type rssEntry struct {
	ID struct {
		Label string `json:"label"`
	} `json:"id"`
	Title struct {
		Label string `json:"label"`
	} `json:"title"`
	Content struct {
		Label string `json:"label"`
	} `json:"content"`
	ImRating struct {
		Label string `json:"label"`
	} `json:"im:rating"`
	ImVersion struct {
		Label string `json:"label"`
	} `json:"im:version"`
	Author struct {
		Name struct {
			Label string `json:"label"`
		} `json:"name"`
		URI struct {
			Label string `json:"label"`
		} `json:"uri"`
	} `json:"author"`
	Updated struct {
		Label string `json:"label"`
	} `json:"updated"`
}

// rssEntries handles the Apple quirk where `feed.entry` is rendered as a
// single object instead of an array when there's exactly one entry on the
// page. Without this normalization, low-volume apps would fail to decode
// and the whole poll would error.
type rssEntries []rssEntry

func (e *rssEntries) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] == '[' {
		var arr []rssEntry
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*e = arr
		return nil
	}
	var one rssEntry
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	*e = []rssEntry{one}
	return nil
}

// rssResponse mirrors the Apple iTunes customer-reviews RSS JSON shape.
type rssResponse struct {
	Feed struct {
		Author struct {
			Name struct {
				Label string `json:"label"`
			} `json:"name"`
		} `json:"author"`
		Entry rssEntries `json:"entry"`
	} `json:"feed"`
}

// FetchPage fetches a single page of reviews from the Apple RSS feed.
// Returns parsed reviews, the resolved app name (only non-empty on page 1), and any error.
func FetchPage(ctx context.Context, rssURL string, page int) ([]store.Review, string, error) {
	url := pageURL(rssURL, page)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "RunwayReviewsBot/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("feed returned HTTP %d for %s", resp.StatusCode, url)
	}

	// Cap the body so a misbehaving upstream can't exhaust memory inside
	// the 10s client timeout.
	body := io.LimitReader(resp.Body, maxFeedBodyBytes)
	var rss rssResponse
	if err := json.NewDecoder(body).Decode(&rss); err != nil {
		return nil, "", fmt.Errorf("decode RSS JSON: %w", err)
	}

	appName := rss.Feed.Author.Name.Label

	var reviews []store.Review
	for _, entry := range rss.Feed.Entry {
		score, err := strconv.Atoi(entry.ImRating.Label)
		if err != nil {
			// Skip entries without a numeric rating (app-info entry on some feeds).
			continue
		}

		submittedAt, err := time.Parse(time.RFC3339, entry.Updated.Label)
		if err != nil {
			// Drop the entry entirely. A zero-time review would never appear
			// in any windowed query AND would poison FetchAll's per-page
			// early-stop heuristic: if it sorts last on a page,
			// oldest.Before(since) is true for any non-zero `since` and
			// pagination terminates prematurely, silently losing newer
			// reviews on later pages. Log so a feed-format regression is
			// at least visible to an operator.
			log.Printf("[package:feed][method:FetchPage][message:dropping entry %s with unparseable updated %q: %v]", entry.ID.Label, entry.Updated.Label, err)
			continue
		}

		reviews = append(reviews, store.Review{
			ID:          extractReviewID(entry.ID.Label),
			Score:       score,
			Title:       sanitize(entry.Title.Label, 512),
			Body:        sanitize(entry.Content.Label, 8192),
			Author:      sanitize(entry.Author.Name.Label, 256),
			Version:     sanitize(entry.ImVersion.Label, 32),
			SubmittedAt: submittedAt.UTC(),
		})
	}

	return reviews, appName, nil
}

// FetchAll fetches pages until it has collected all reviews or hits MaxPages.
// FetchAll walks pages 1..MaxPages until Apple returns empty. Caller
// dedupes by review ID. No early-stop: a since-based cutoff would
// permanently block backfill if the first poll ever truncated.
func FetchAll(ctx context.Context, rssURL string) ([]store.Review, string, error) {
	var all []store.Review
	var appName string

	hitCap := true
	for page := 1; page <= MaxPages; page++ {
		if ctx.Err() != nil {
			return all, appName, ctx.Err()
		}

		reviews, name, err := FetchPage(ctx, rssURL, page)
		if err != nil {
			return all, appName, fmt.Errorf("page %d: %w", page, err)
		}
		if name != "" {
			appName = name
		}
		if len(reviews) == 0 {
			hitCap = false
			break
		}
		all = append(all, reviews...)
	}

	// If we iterated all MaxPages without an empty page, Apple may have
	// more history than we can reach. Surface it so an operator can spot
	// apps that consistently outrun the cap.
	if hitCap {
		log.Printf("[package:feed][method:FetchAll][message:MaxPages=%d cap hit for %s — history may be truncated]", MaxPages, rssURL)
	}

	return all, appName, nil
}

// pageURL inserts the page number into an Apple RSS URL.
// Input:  https://itunes.apple.com/us/rss/customerreviews/id=595068606/sortBy=mostRecent/json
// Output: https://itunes.apple.com/us/rss/customerreviews/id=595068606/sortBy=mostRecent/page=2/json
func pageURL(rssURL string, page int) string {
	if page <= 1 {
		return ensureJSON(rssURL)
	}
	base := ensureJSON(rssURL)
	if idx := strings.LastIndex(base, "/json"); idx != -1 {
		return base[:idx] + fmt.Sprintf("/page=%d", page) + base[idx:]
	}
	return fmt.Sprintf("%s/page=%d/json", strings.TrimRight(base, "/"), page)
}

// ensureJSON ensures the URL ends with /json (not /xml).
func ensureJSON(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/xml") {
		u = u[:len(u)-4] + "/json"
	} else if !strings.HasSuffix(u, "/json") {
		u += "/json"
	}
	return u
}

// extractReviewID pulls the numeric ID out of an iTunes review URL like:
// https://itunes.apple.com/us/review/id123456789?type=Purple+Software
func extractReviewID(label string) string {
	label = strings.SplitN(label, "?", 2)[0]
	parts := strings.Split(strings.TrimRight(label, "/"), "/")
	last := parts[len(parts)-1]
	return strings.TrimPrefix(last, "id")
}

// sanitize strips null bytes and non-printable control characters from RSS
// text fields, then truncates to maxLen runes. Newlines and tabs are preserved
// since they appear in legitimate review bodies.
func sanitize(s string, maxLen int) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\x00' {
			continue
		}
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	result := strings.TrimSpace(b.String())
	runes := []rune(result)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return result
}
