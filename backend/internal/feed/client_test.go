package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPageURL_Page1(t *testing.T) {
	base := "https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/json"
	got := pageURL(base, 1)
	if got != base {
		t.Errorf("page 1 should return base URL unchanged, got %q", got)
	}
}

func TestPageURL_Page2(t *testing.T) {
	base := "https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/json"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/page=2/json"
	got := pageURL(base, 2)
	if got != want {
		t.Errorf("pageURL(base, 2) = %q, want %q", got, want)
	}
}

func TestPageURL_Page10(t *testing.T) {
	base := "https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/json"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/page=10/json"
	got := pageURL(base, 10)
	if got != want {
		t.Errorf("pageURL(base, 10) = %q, want %q", got, want)
	}
}

func TestEnsureJSON_AlreadyJSON(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/json"
	if got := ensureJSON(u); got != u {
		t.Errorf("ensureJSON(%q) = %q, expected no change", u, got)
	}
}

func TestEnsureJSON_XML(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/xml"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/json"
	if got := ensureJSON(u); got != want {
		t.Errorf("ensureJSON(%q) = %q, want %q", u, got, want)
	}
}

func TestEnsureJSON_NoExtension(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/json"
	if got := ensureJSON(u); got != want {
		t.Errorf("ensureJSON(%q) = %q, want %q", u, got, want)
	}
}

func TestExtractReviewID_FullURL(t *testing.T) {
	label := "https://itunes.apple.com/us/review/id123456789?type=Purple+Software"
	want := "123456789"
	if got := extractReviewID(label); got != want {
		t.Errorf("extractReviewID(%q) = %q, want %q", label, got, want)
	}
}

func TestExtractReviewID_NoIDPrefix(t *testing.T) {
	label := "https://itunes.apple.com/us/review/987654321"
	want := "987654321"
	if got := extractReviewID(label); got != want {
		t.Errorf("extractReviewID(%q) = %q, want %q", label, got, want)
	}
}

func TestExtractReviewID_PlainID(t *testing.T) {
	label := "id555"
	want := "555"
	if got := extractReviewID(label); got != want {
		t.Errorf("extractReviewID(%q) = %q, want %q", label, got, want)
	}
}

// ---- sanitize ---------------------------------------------------------------

func TestSanitize_StripsNullBytes(t *testing.T) {
	got := sanitize("hello\x00world", 256)
	if strings.Contains(got, "\x00") {
		t.Errorf("expected null byte stripped, got %q", got)
	}
	if got != "helloworld" {
		t.Errorf("sanitize null byte: got %q, want %q", got, "helloworld")
	}
}

func TestSanitize_StripsControlChars(t *testing.T) {
	// \x01 through \x1f except \t \n \r should be stripped.
	got := sanitize("a\x01b\x02c", 256)
	if got != "abc" {
		t.Errorf("sanitize control chars: got %q, want %q", got, "abc")
	}
}

func TestSanitize_PreservesNewlineAndTab(t *testing.T) {
	input := "line1\nline2\ttabbed"
	got := sanitize(input, 256)
	if got != input {
		t.Errorf("sanitize should preserve newline/tab: got %q, want %q", got, input)
	}
}

func TestSanitize_TruncatesAtMaxLen(t *testing.T) {
	input := strings.Repeat("a", 600)
	got := sanitize(input, 512)
	if len([]rune(got)) != 512 {
		t.Errorf("expected truncation to 512 runes, got %d", len([]rune(got)))
	}
}

func TestSanitize_TrimsWhitespace(t *testing.T) {
	got := sanitize("  hello  ", 256)
	if got != "hello" {
		t.Errorf("sanitize trim: got %q, want %q", got, "hello")
	}
}

// ---- FetchPage (HTTP integration with mock server) --------------------------

// mockRSSJSON builds a minimal Apple RSS JSON response for use in tests.
func mockRSSJSON(appName string, entries []map[string]any) []byte {
	feed := map[string]any{
		"feed": map[string]any{
			"author": map[string]any{
				"name": map[string]any{"label": appName},
			},
			"entry": entries,
		},
	}
	b, _ := json.Marshal(feed)
	return b
}

// reviewEntry constructs a single feed entry map matching the Apple RSS shape.
func reviewEntry(id, title, body, rating, version, author, updated string) map[string]any {
	return map[string]any{
		"id":         map[string]any{"label": id},
		"title":      map[string]any{"label": title},
		"content":    map[string]any{"label": body},
		"im:rating":  map[string]any{"label": rating},
		"im:version": map[string]any{"label": version},
		"author": map[string]any{
			"name": map[string]any{"label": author},
			"uri":  map[string]any{"label": ""},
		},
		"updated": map[string]any{"label": updated},
	}
}

func TestFetchPage_ParsesReviews(t *testing.T) {
	body := mockRSSJSON("Snapchat", []map[string]any{
		reviewEntry(
			"https://itunes.apple.com/us/review/id111111111?type=Purple+Software",
			"Excellent app", "Really love it", "5", "12.3.0", "happyuser",
			"2024-05-01T10:00:00-07:00",
		),
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	reviews, appName, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if appName != "Snapchat" {
		t.Errorf("appName = %q, want %q", appName, "Snapchat")
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	r := reviews[0]
	if r.ID != "111111111" {
		t.Errorf("ID = %q, want %q", r.ID, "111111111")
	}
	if r.Score != 5 {
		t.Errorf("Score = %d, want 5", r.Score)
	}
	if r.Title != "Excellent app" {
		t.Errorf("Title = %q, want %q", r.Title, "Excellent app")
	}
	if r.Body != "Really love it" {
		t.Errorf("Body = %q, want %q", r.Body, "Really love it")
	}
	if r.Author != "happyuser" {
		t.Errorf("Author = %q, want %q", r.Author, "happyuser")
	}
	if r.Version != "12.3.0" {
		t.Errorf("Version = %q, want %q", r.Version, "12.3.0")
	}
	if r.SubmittedAt.IsZero() {
		t.Error("SubmittedAt should not be zero")
	}
	if r.SubmittedAt.Location() != time.UTC {
		t.Error("SubmittedAt should be normalised to UTC")
	}
}

// Apple RSS feeds include a non-review app-info entry at index 0 on some pages;
// it has a non-numeric im:rating. FetchPage must silently skip it.
func TestFetchPage_SkipsNonNumericRatingEntry(t *testing.T) {
	body := mockRSSJSON("TestApp", []map[string]any{
		reviewEntry("https://itunes.apple.com/us/app/id123", "AppInfo", "desc", "Not a number", "", "AppName", "2024-01-01T00:00:00Z"),
		reviewEntry("https://itunes.apple.com/us/review/id999", "Real review", "body", "4", "1.0", "user", "2024-01-01T00:00:00Z"),
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	reviews, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if len(reviews) != 1 {
		t.Errorf("expected 1 review (non-numeric rating entry skipped), got %d", len(reviews))
	}
}

func TestFetchPage_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	_, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

// The HTTP client has CheckRedirect disabled to block SSRF: a crafted RSS URL
// could redirect to an internal-network resource or cloud metadata endpoint.
// A 3xx must not be followed — it must surface as an error.
func TestFetchPage_SSRFRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer ts.Close()

	_, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	// CheckRedirect returns ErrUseLastResponse so the 302 is surfaced as a
	// non-200 status; we just need an error — the exact message is an
	// implementation detail and must not be asserted.
	if err == nil {
		t.Error("expected error: redirect must not be followed (SSRF prevention)")
	}
}

func TestFetchPage_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json {{{"))
	}))
	defer ts.Close()

	_, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err == nil {
		t.Error("expected error for invalid JSON response body")
	}
}

// ---- FetchAll (pagination + early-stop + context) ---------------------------

// FetchAll must stop when a page returns an empty entry list.
func TestFetchAll_StopsOnEmptyPage(t *testing.T) {
	pageCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		if pageCount >= 2 {
			_, _ = w.Write(mockRSSJSON("App", []map[string]any{}))
			return
		}
		recent := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		_, _ = w.Write(mockRSSJSON("App", []map[string]any{
			reviewEntry("https://itunes.apple.com/us/review/id1", "Title", "body", "5", "1.0", "user", recent),
		}))
	}))
	defer ts.Close()

	_, _, err := FetchAll(context.Background(), ts.URL+"/json")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if pageCount > 2 {
		t.Errorf("FetchAll made %d page requests after receiving an empty page", pageCount)
	}
}

// FetchAll checks ctx.Err() at the top of each page iteration, so a
// pre-cancelled context must return immediately without hitting the network.
func TestFetchAll_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := FetchAll(ctx, "http://127.0.0.1:0/json")
	if err == nil {
		t.Error("expected error for pre-cancelled context")
	}
}

// --- Tests for rssEntries.UnmarshalJSON ---
// Apple renders feed.entry as an array for most pages but as a single bare
// object when there's exactly one entry. The custom unmarshaller normalises
// both shapes into a slice so callers don't have to branch.

func TestRssEntries_Array(t *testing.T) {
	data := []byte(`[{"id":{"label":"a"}},{"id":{"label":"b"}}]`)
	var got rssEntries
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if len(got) != 2 || got[0].ID.Label != "a" || got[1].ID.Label != "b" {
		t.Errorf("got %+v, want 2 entries [a,b]", got)
	}
}

func TestRssEntries_SingleObject(t *testing.T) {
	data := []byte(`{"id":{"label":"only"}}`)
	var got rssEntries
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal single: %v", err)
	}
	if len(got) != 1 || got[0].ID.Label != "only" {
		t.Errorf("got %+v, want single entry [only]", got)
	}
}

func TestRssEntries_Null(t *testing.T) {
	data := []byte(`null`)
	var got rssEntries
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want empty for null", got)
	}
}

func TestRssEntries_EmptyArray(t *testing.T) {
	data := []byte(`[]`)
	var got rssEntries
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal empty array: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want empty slice", got)
	}
}

func TestRssEntries_Malformed(t *testing.T) {
	data := []byte(`{not json}`)
	var got rssEntries
	if err := json.Unmarshal(data, &got); err == nil {
		t.Error("expected error decoding garbage, got nil")
	}
}

// FetchPage end-to-end through the single-object path: low-volume apps
// (1 review on the page) used to fail entirely because the embedded shape
// rejected an object where a slice was expected.
func TestFetchPage_AcceptsSingleObjectEntry(t *testing.T) {
	body := `{"feed":{"author":{"name":{"label":"App"}},"entry":` +
		`{"id":{"label":"https://itunes.apple.com/us/review/id1?type=Purple+Software"},` +
		`"title":{"label":"x"},"content":{"label":"y"},"im:rating":{"label":"4"},` +
		`"im:version":{"label":"1"},"author":{"name":{"label":"a"},"uri":{"label":""}},` +
		`"updated":{"label":"2024-01-01T00:00:00Z"}}}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	reviews, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Score != 4 {
		t.Errorf("expected 1 review with score 4, got %+v", reviews)
	}
}

// --- Tests for CanonicalizeURL ---
// The poller's pagination assumes /sortBy=mostRecent/, so the handler
// canonicalises before storing. These tests pin the rewrite rules.

func TestCanonicalizeURL_AlreadyCanonical(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=mostRecent/json"
	if got := CanonicalizeURL(u); got != u {
		t.Errorf("CanonicalizeURL(%q) = %q, want unchanged", u, got)
	}
}

func TestCanonicalizeURL_MissingSortBy(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/json"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=mostRecent/json"
	if got := CanonicalizeURL(u); got != want {
		t.Errorf("CanonicalizeURL(%q) = %q, want %q", u, got, want)
	}
}

func TestCanonicalizeURL_DifferentSortBy(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=helpful/json"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=mostRecent/json"
	if got := CanonicalizeURL(u); got != want {
		t.Errorf("CanonicalizeURL(%q) = %q, want %q", u, got, want)
	}
}

func TestCanonicalizeURL_NoJSONSuffix(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=mostRecent/json"
	if got := CanonicalizeURL(u); got != want {
		t.Errorf("CanonicalizeURL(%q) = %q, want %q", u, got, want)
	}
}

// CanonicalizeURL must strip a pre-existing /page=N/ segment — the example
// URL in the assignment ends in /page=1/, and without this strip pageURL
// would produce /page=1/page=2/json on the next poll.
func TestCanonicalizeURL_StripsExistingPage(t *testing.T) {
	u := "https://itunes.apple.com/us/rss/customerreviews/id=123/page=1/sortBy=mostRecent/json"
	want := "https://itunes.apple.com/us/rss/customerreviews/id=123/sortBy=mostRecent/json"
	if got := CanonicalizeURL(u); got != want {
		t.Errorf("CanonicalizeURL(%q) = %q, want %q", u, got, want)
	}
}

// FetchPage must drop reviews with an unparseable `updated` timestamp.
// Persisting them as time.Time{} would poison FetchAll's per-page
// early-stop heuristic and cause silent data loss on subsequent pages.
func TestFetchPage_DropsUnparseableTimestamp(t *testing.T) {
	body := `{"feed":{"author":{"name":{"label":"App"}},"entry":[` +
		`{"id":{"label":"id1"},"title":{"label":"t"},"content":{"label":"c"},` +
		`"im:rating":{"label":"5"},"im:version":{"label":"1"},` +
		`"author":{"name":{"label":"a"},"uri":{"label":""}},` +
		`"updated":{"label":"not a date"}},` +
		`{"id":{"label":"id2"},"title":{"label":"t"},"content":{"label":"c"},` +
		`"im:rating":{"label":"4"},"im:version":{"label":"1"},` +
		`"author":{"name":{"label":"a"},"uri":{"label":""}},` +
		`"updated":{"label":"2024-01-01T00:00:00Z"}}]}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	reviews, _, err := FetchPage(context.Background(), ts.URL+"/json", 1)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 valid review (the bad-timestamp entry dropped), got %d: %+v", len(reviews), reviews)
	}
	if reviews[0].SubmittedAt.IsZero() {
		t.Errorf("returned review has zero time, expected the parseable one")
	}
}

// --- LookupAppName tests ----------------------------------------------------
// lookupBaseURL is swapped to point at a httptest server so these tests
// stay hermetic (no real itunes.apple.com calls).

func withLookupBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := lookupBaseURL
	lookupBaseURL = url
	t.Cleanup(func() { lookupBaseURL = prev })
}

func TestLookupAppName_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resultCount":1,"results":[{"trackName":"Notion – Notes, Docs, Tasks"}]}`))
	}))
	defer ts.Close()
	withLookupBaseURL(t, ts.URL+"?id=")

	got := LookupAppName(context.Background(), "1232780281")
	if got != "Notion – Notes, Docs, Tasks" {
		t.Errorf("LookupAppName = %q, want %q", got, "Notion – Notes, Docs, Tasks")
	}
}

// A non-200 response must NOT leak the connection — defer Close is
// registered before the status check, so the body is always closed.
func TestLookupAppName_Non200ReturnsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	withLookupBaseURL(t, ts.URL+"?id=")

	if got := LookupAppName(context.Background(), "1"); got != "" {
		t.Errorf("LookupAppName on 500 = %q, want empty", got)
	}
}

func TestLookupAppName_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resultCount":0,"results":[]}`))
	}))
	defer ts.Close()
	withLookupBaseURL(t, ts.URL+"?id=")

	if got := LookupAppName(context.Background(), "1"); got != "" {
		t.Errorf("LookupAppName with empty results = %q, want empty", got)
	}
}

func TestLookupAppName_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json}`))
	}))
	defer ts.Close()
	withLookupBaseURL(t, ts.URL+"?id=")

	if got := LookupAppName(context.Background(), "1"); got != "" {
		t.Errorf("LookupAppName on malformed JSON = %q, want empty", got)
	}
}

func TestLookupAppName_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer ts.Close()
	withLookupBaseURL(t, ts.URL+"?id=")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := LookupAppName(ctx, "1"); got != "" {
		t.Errorf("LookupAppName with cancelled ctx = %q, want empty", got)
	}
}

// FetchAll must walk every page, not stop after page 1 just because the
// reviews are recent. Regression for an app stuck at 50 reviews.
func TestFetchAll_WalksAllPagesEvenWhenOldestRecent(t *testing.T) {
	pagesHit := map[string]bool{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "page=2"):
			pagesHit["2"] = true
		case strings.Contains(r.URL.Path, "page=3"):
			pagesHit["3"] = true
			// Empty terminates the loop.
			_, _ = w.Write(mockRSSJSON("App", nil))
			return
		default:
			pagesHit["1"] = true
		}
		// All pages return reviews from the last hour — under the old
		// early-stop, anything but a zero `since` would have stopped at
		// page 1. With the early-stop removed we keep going.
		recent := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
		_, _ = w.Write(mockRSSJSON("App", []map[string]any{
			reviewEntry("https://itunes.apple.com/us/review/id"+r.URL.Path, "T", "b", "5", "1.0", "user", recent),
		}))
	}))
	defer ts.Close()

	_, _, err := FetchAll(context.Background(), ts.URL+"/json")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if !pagesHit["1"] || !pagesHit["2"] || !pagesHit["3"] {
		t.Errorf("expected pages 1, 2, 3 all hit; got %v", pagesHit)
	}
}
