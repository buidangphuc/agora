package edge

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
)

func postTrack(t *testing.T, e *Edge, pub *mockAnalyticsPublisher, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := HandleTrack(e, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func counts(t *testing.T, rec *httptest.ResponseRecorder) (int, int) {
	t.Helper()
	var r trackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return r.Accepted, r.Dropped
}

func TestTrackMixedBatchDropsOnlyInvalid(t *testing.T) {
	pub := &mockAnalyticsPublisher{}
	rec := postTrack(t, &Edge{}, pub, `[{"type":"view","listingId":"a"},{"type":"teleport","listingId":"b"},{"type":"click","listingId":"c"}]`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code %d", rec.Code)
	}
	if a, d := counts(t, rec); a != 2 || d != 1 {
		t.Fatalf("accepted %d dropped %d", a, d)
	}
	if len(pub.events) != 2 || pub.events[0].GetListingId() != "a" || pub.events[1].GetListingId() != "c" {
		t.Fatalf("published %v", pub.events)
	}
}

func TestTrackNoValidEventIs400AndProducesNothing(t *testing.T) {
	pub := &mockAnalyticsPublisher{}
	for _, body := range []string{`[{"type":"teleport"}]`, `{"type":""}`, `not json`, `[]`} {
		if rec := postTrack(t, &Edge{}, pub, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: code %d", body, rec.Code)
		}
	}
	if len(pub.events) != 0 {
		t.Fatalf("produced %d", len(pub.events))
	}
}

func TestTrackBounds(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	props := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = `"k` + strings.Repeat("x", i) + `":"v"`
		}
		return `{` + strings.Join(parts, ",") + `}`
	}
	cases := []struct {
		name  string
		extra string
		valid bool
	}{
		{"id at limit", `"listingId":"` + long(128) + `"`, true},
		{"listingId over", `"listingId":"` + long(129) + `"`, false},
		{"sessionId over", `"sessionId":"` + long(129) + `"`, false},
		{"anonymousId over", `"anonymousId":"` + long(129) + `"`, false},
		{"placementId over", `"placementId":"` + long(129) + `"`, false},
		{"impressionId over", `"impressionId":"` + long(129) + `"`, false},
		{"modelVersion over", `"modelVersion":"` + long(129) + `"`, false},
		{"eventGroupId over", `"eventGroupId":"` + long(129) + `"`, false},
		{"transactionId over", `"transactionId":"` + long(129) + `"`, false},
		{"path at limit", `"path":"` + long(512) + `"`, true},
		{"path over", `"path":"` + long(513) + `"`, false},
		{"referrer over", `"referrer":"` + long(513) + `"`, false},
		{"query at limit", `"query":"` + long(256) + `"`, true},
		{"query over", `"query":"` + long(257) + `"`, false},
		{"20 props", `"properties":` + props(20), true},
		{"21 props", `"properties":` + props(21), false},
		{"key at limit", `"properties":{"` + long(40) + `":"v"}`, true},
		{"key over", `"properties":{"` + long(41) + `":"v"}`, false},
		{"value at limit", `"properties":{"k":"` + long(256) + `"}`, true},
		{"value over", `"properties":{"k":"` + long(257) + `"}`, false},
	}
	for _, c := range cases {
		pub := &mockAnalyticsPublisher{}
		// a valid companion keeps the batch from being a 400
		rec := postTrack(t, &Edge{}, pub, `[{"type":"view","listingId":"ok"},{"type":"view",`+c.extra+`}]`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s: code %d", c.name, rec.Code)
		}
		a, d := counts(t, rec)
		want := 1
		if c.valid {
			want = 2
		}
		if a != want || a+d != 2 {
			t.Errorf("%s: accepted %d dropped %d", c.name, a, d)
		}
	}
}

func TestTrackEventIDDerivation(t *testing.T) {
	id := uuid.NewString()
	other := uuid.NewString()
	pub := &mockAnalyticsPublisher{}
	postTrack(t, &Edge{}, pub, `[
{"type":"view","eventId":"`+id+`","anonymousId":"A"},
{"type":"view","eventId":"`+id+`","anonymousId":"A"},
{"type":"view","eventId":"`+id+`","anonymousId":"B"},
{"type":"view","eventId":"`+other+`","anonymousId":"A"},
{"type":"view","anonymousId":"A"},
{"type":"view","anonymousId":"A"},
{"type":"view","eventId":"not-a-uuid","anonymousId":"A"},
{"type":"view","eventId":"`+id+`"}]`)
	ids := pub.ids
	if len(ids) != 8 {
		t.Fatalf("ids %v", ids)
	}
	if ids[0] == "" || ids[0] != ids[1] {
		t.Errorf("same visitor same eventId must match: %q %q", ids[0], ids[1])
	}
	if ids[0] == ids[2] {
		t.Error("other visitor must differ")
	}
	if ids[0] == ids[3] {
		t.Error("other eventId must differ")
	}
	if ids[4] != "" || ids[5] != "" || ids[6] != "" || ids[7] != "" {
		t.Errorf("no/invalid eventId or no visitor must mint random (empty here): %v", ids[4:])
	}
	if _, err := uuid.Parse(ids[0]); err != nil {
		t.Errorf("not a uuid: %v", err)
	}
}

func TestTrackEventIDScopedToVerifiedPrincipal(t *testing.T) {
	id := uuid.NewString()
	e := &Edge{}
	b := &trackBeacon{EventID: id, AnonymousID: "A"}
	anon := e.beaconPrincipal(httptest.NewRequest(http.MethodPost, "/api/track", nil))
	got := envelopeEventID(b, anon)
	user := &commonv1.Principal{Id: "user-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
	if u := envelopeEventID(b, user); u == "" || u == got {
		t.Errorf("user key must differ from anon key: %q vs %q", u, got)
	}
	// a spoofed anonymousId cannot reproduce a user's id
	b2 := &trackBeacon{EventID: id, AnonymousID: "user-1"}
	if envelopeEventID(b2, anon) == envelopeEventID(b, user) {
		t.Error("anon:<x> must not collide with principal <x>")
	}
}

func TestScrubText(t *testing.T) {
	cases := map[string]string{
		"a.b@example.com":                  "[email]",
		"mail a.b+x@sub.example.co.uk now": "mail [email] now",
		"0912345678":                       "[phone]",
		"0912 345 678":                     "[phone]",
		"0912.345.678":                     "[phone]",
		"+84 912 345 678":                  "[phone]",
		"+84912345678":                     "[phone]",
		"1234567890123":                    "[number]",
		"12345678901234567":                "[number]",
		"850000":                           "850000",
		"giá 1500000000 đ":                 "giá 1500000000 đ",
		"listing 123456":                   "listing 123456",
		"tủ lạnh":                          "tủ lạnh",
		"":                                 "",
	}
	for in, want := range cases {
		if got := scrubText(in); got != want {
			t.Errorf("scrubText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := scrubText("a.b@example.com 0912 345 678 tủ lạnh 850000"); got != "[email] [phone] tủ lạnh 850000" {
		t.Errorf("spec scenario: %q", got)
	}
}

func TestScrubReferrer(t *testing.T) {
	cases := map[string]string{
		"https://news.example.com/a/b?utm_source=x&email=a@b.co#top": "https://news.example.com/a/b",
		"https://example.com": "https://example.com",
		"/home":               "",
		"://bad":              "",
		"":                    "",
	}
	for in, want := range cases {
		if got := scrubReferrer(in); got != want {
			t.Errorf("scrubReferrer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrackScrubsPublishedFields(t *testing.T) {
	pub := &mockAnalyticsPublisher{}
	postTrack(t, &Edge{}, pub, `{"type":"view","query":"a@b.co 0912345678","path":"/s?q=0912345678","referrer":"https://x.io/p?e=a@b.co#h","properties":{"k":"a@b.co"}}`)
	ev := pub.events[0]
	if ev.GetSearchQuery() != "[email] [phone]" || ev.GetPagePath() != "/s?q=[phone]" ||
		ev.GetReferrer() != "https://x.io/p" || ev.GetProperties()["k"] != "[email]" {
		t.Fatalf("not scrubbed: %+v", ev)
	}
}
