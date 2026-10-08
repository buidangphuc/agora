package edge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/events"
)

// maxBeaconBytes caps the request body a browser beacon may send. Telemetry is
// small; anything larger is a malformed/abusive request.
const maxBeaconBytes = 64 * 1024

// maxBatchItems caps the number of events sent in a single batched array.
const maxBatchItems = 100

// trackBeacon is the browser beacon shape (see team-frontend/src/lib/track.ts).
// It carries behavioral context ONLY — never authenticated identity, which the
// edge attaches via the envelope principal (contract forbids PII in payload).
type trackBeacon struct {
	Type          string            `json:"type"`
	ListingID     string            `json:"listingId"`
	SessionID     string            `json:"sessionId"`
	AnonymousID   string            `json:"anonymousId"`
	Path          string            `json:"path"`
	Referrer      string            `json:"referrer"`
	Position      uint32            `json:"position"`
	Query         string            `json:"query"`
	PlacementID   string            `json:"placementId"`
	ImpressionID  string            `json:"impressionId"`
	ModelVersion  string            `json:"modelVersion"`
	Properties    map[string]string `json:"properties"`
	Currency      string            `json:"currency"`
	Value         int64             `json:"value"`
	Price         int64             `json:"price"`
	Quantity      uint32            `json:"quantity"`
	TransactionID string            `json:"transactionId"`
	Coupon        string            `json:"coupon"`
	ItemCategory  string            `json:"itemCategory"`
	ItemListID    string            `json:"itemListId"`
	ItemListName  string            `json:"itemListName"`
	EventGroupID  string            `json:"eventGroupId"`
	ShippingTier  string            `json:"shippingTier"`
	PaymentType   string            `json:"paymentType"`
	EventID       string            `json:"eventId"`
}

// beaconEventTypes maps the beacon's lowercase action name to its EventType.
// An unknown/empty type is rejected (nothing is produced).
var beaconEventTypes = map[string]analyticsv1.EventType{
	"view":              analyticsv1.EventType_EVENT_TYPE_VIEW,
	"click":             analyticsv1.EventType_EVENT_TYPE_CLICK,
	"add_to_cart":       analyticsv1.EventType_EVENT_TYPE_ADD_TO_CART,
	"impression":        analyticsv1.EventType_EVENT_TYPE_IMPRESSION,
	"remove_from_cart":  analyticsv1.EventType_EVENT_TYPE_REMOVE_FROM_CART,
	"begin_checkout":    analyticsv1.EventType_EVENT_TYPE_BEGIN_CHECKOUT,
	"apply_promotion":   analyticsv1.EventType_EVENT_TYPE_APPLY_PROMOTION,
	"search_filter":     analyticsv1.EventType_EVENT_TYPE_SEARCH_FILTER,
	"favorite":          analyticsv1.EventType_EVENT_TYPE_FAVORITE,
	"share":             analyticsv1.EventType_EVENT_TYPE_SHARE,
	"view_cart":         analyticsv1.EventType_EVENT_TYPE_VIEW_CART,
	"add_shipping_info": analyticsv1.EventType_EVENT_TYPE_ADD_SHIPPING_INFO,
	"add_payment_info":  analyticsv1.EventType_EVENT_TYPE_ADD_PAYMENT_INFO,
	"purchase":          analyticsv1.EventType_EVENT_TYPE_PURCHASE,

	// GA4 Standard Event Aliases
	"view_item":      analyticsv1.EventType_EVENT_TYPE_VIEW,
	"select_item":    analyticsv1.EventType_EVENT_TYPE_CLICK,
	"view_item_list": analyticsv1.EventType_EVENT_TYPE_IMPRESSION,
}

// Per-field bounds (characters). An event breaking any of them is dropped.
const (
	maxIDChars        = 128 // listingId, sessionId, anonymousId, placementId, ...
	maxPathChars      = 512 // path, referrer
	maxQueryChars     = 256 // query
	maxPropKeys       = 20
	maxPropKeyChars   = 40
	maxPropValueChars = 256
)

// nsTrack is the fixed UUID namespace for deterministic envelope event ids.
var nsTrack = uuid.MustParse("6f0d5f6e-3c1b-5a43-9a7e-2b8c4d1e9f10")

var (
	reEmail     = regexp.MustCompile(`[\w.+-]+@[\w-]+(\.[\w-]+)+`)
	rePhone     = regexp.MustCompile(`(?:\+84|\b0)(?:[\s.]?\d){9}\b`)
	reLongDigit = regexp.MustCompile(`\d{13,}`)
)

// scrubText masks personal data in free text: emails, VN phone numbers and long
// digit runs. Prices, listing ids and other short numbers are kept.
func scrubText(s string) string {
	s = reEmail.ReplaceAllString(s, "[email]")
	s = rePhone.ReplaceAllString(s, "[phone]")
	return reLongDigit.ReplaceAllString(s, "[number]")
}

// scrubReferrer reduces a referrer to scheme://host/path. A value that does not
// parse to an absolute URL becomes empty.
func scrubReferrer(ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// validateBeacon applies the per-event type and bounds rules.
func validateBeacon(b *trackBeacon) error {
	if _, ok := beaconEventTypes[strings.ToLower(strings.TrimSpace(b.Type))]; !ok {
		return jsonError("unknown event type")
	}
	for _, f := range []string{b.ListingID, b.SessionID, b.AnonymousID, b.PlacementID, b.ImpressionID,
		b.ModelVersion, b.EventGroupID, b.TransactionID} {
		if utf8.RuneCountInString(f) > maxIDChars {
			return jsonError("field too long")
		}
	}
	if utf8.RuneCountInString(b.Path) > maxPathChars || utf8.RuneCountInString(b.Referrer) > maxPathChars {
		return jsonError("path too long")
	}
	if utf8.RuneCountInString(b.Query) > maxQueryChars {
		return jsonError("query too long")
	}
	if len(b.Properties) > maxPropKeys {
		return jsonError("too many properties")
	}
	for k, v := range b.Properties {
		if utf8.RuneCountInString(k) > maxPropKeyChars || utf8.RuneCountInString(v) > maxPropValueChars {
			return jsonError("property too long")
		}
	}
	return nil
}

// envelopeEventID derives the deterministic envelope event_id from the visitor
// and the client-supplied eventId. Empty means "mint a random one": no valid
// UUID eventId, or no visitor key (anonymous without an anonymousId).
func envelopeEventID(b *trackBeacon, p *commonv1.Principal) string {
	if _, err := uuid.Parse(b.EventID); err != nil {
		return ""
	}
	var visitor string
	switch {
	case p.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS && p.GetId() != "":
		visitor = p.GetId()
	case b.AnonymousID != "":
		visitor = "anon:" + b.AnonymousID
	default:
		return ""
	}
	return uuid.NewSHA1(nsTrack, []byte(visitor+"|"+strings.ToLower(b.EventID))).String()
}

// trackResponse is the 202 body of POST /api/track.
type trackResponse struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}

// HandleTrack builds the pure edge-telemetry collector: parse the beacon (single
// or a small batch), validate each event on its own (invalid ones are dropped),
// scrub free text, map each beacon to a TrackingEvent, stamp the edge-resolved
// principal on the envelope, and forward to Kafka. It answers 202 with the
// accepted/dropped counts, or 400 when nothing in the body is valid. It holds
// no business logic and owns no analytics storage (Rule 2). Delivery is
// best-effort: a produce error is logged and the beacon still counts as accepted
// so a dropped beacon never surfaces to the user.
func HandleTrack(e *Edge, pub events.AnalyticsPublisher, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBeaconBytes))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}

		beacons, err := parseBeacons(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		principal := e.beaconPrincipal(r)

		type item struct {
			ev      *analyticsv1.TrackingEvent
			eventID string
		}
		items := make([]item, 0, len(beacons))
		for i := range beacons {
			b := &beacons[i]
			if err := validateBeacon(b); err != nil {
				continue
			}
			et := beaconEventTypes[strings.ToLower(strings.TrimSpace(b.Type))]
			var props map[string]string
			if b.Properties != nil {
				props = make(map[string]string, len(b.Properties))
				for k, v := range b.Properties {
					props[k] = scrubText(v)
				}
			}
			items = append(items, item{
				eventID: envelopeEventID(b, principal),
				ev: &analyticsv1.TrackingEvent{
					EventType:     et,
					ListingId:     b.ListingID,
					SessionId:     b.SessionID,
					AnonymousId:   b.AnonymousID,
					PagePath:      scrubText(b.Path),
					Referrer:      scrubReferrer(b.Referrer),
					Position:      b.Position,
					SearchQuery:   scrubText(b.Query),
					Properties:    props,
					PlacementId:   b.PlacementID,
					ImpressionId:  b.ImpressionID,
					ModelVersion:  b.ModelVersion,
					Currency:      b.Currency,
					Value:         b.Value,
					Price:         b.Price,
					Quantity:      b.Quantity,
					TransactionId: b.TransactionID,
					Coupon:        b.Coupon,
					ItemCategory:  b.ItemCategory,
					ItemListId:    b.ItemListID,
					ItemListName:  b.ItemListName,
					EventGroupId:  b.EventGroupID,
					ShippingTier:  b.ShippingTier,
					PaymentType:   b.PaymentType,
				},
			})
		}
		if len(items) == 0 {
			http.Error(w, "no valid event", http.StatusBadRequest)
			return
		}

		requestID := requestIDFrom(r.Context())
		if requestID == "" {
			requestID = sanitizeRequestID(strings.TrimSpace(r.Header.Get("X-Request-Id")))
		}

		for _, it := range items {
			if err := pub.PublishTrackingEvent(r.Context(), it.ev, principal, requestID, it.eventID); err != nil {
				// Best-effort: log and keep going; the browsing action must not fail.
				logger.Warn("publish tracking event",
					slog.String("event_type", it.ev.GetEventType().String()),
					slog.String("listing_id", it.ev.GetListingId()),
					slog.Any("err", err),
				)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(trackResponse{Accepted: len(items), Dropped: len(beacons) - len(items)})
	}
}

// parseBeacons accepts either a single beacon object or a small JSON array of
// them. An empty payload or empty array is malformed.
func parseBeacons(body []byte) ([]trackBeacon, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, jsonError("empty body")
	}
	if strings.HasPrefix(trimmed, "[") {
		var batch []trackBeacon
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, jsonError("malformed body")
		}
		if len(batch) == 0 {
			return nil, jsonError("empty batch")
		}
		if len(batch) > maxBatchItems {
			return nil, jsonError("batch exceeds max items")
		}
		return batch, nil
	}
	var single trackBeacon
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, jsonError("malformed body")
	}
	return []trackBeacon{single}, nil
}

type beaconParseError string

func (e beaconParseError) Error() string { return string(e) }

func jsonError(msg string) error { return beaconParseError(msg) }

// beaconPrincipal resolves the caller's Principal for a beacon. A browser
// sendBeacon cannot set an Authorization header, so the edge also honors the
// `session` cookie (the same JWT team-identity signs); no/invalid credential
// yields the anonymous principal with the configured public scopes.
func (e *Edge) beaconPrincipal(r *http.Request) *commonv1.Principal {
	p := &commonv1.Principal{
		Id:     "anonymous",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS,
		Scopes: e.publicScopes,
	}

	tok := bearerToken(r.Header.Get("Authorization"))
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = strings.TrimSpace(c.Value)
		}
	}
	if tok == "" {
		return p
	}

	claims, err := e.verifyToken(tok)
	if err != nil {
		return p
	}
	p.Id = claims.Subject
	p.Type = principalType(claims.Type)
	p.Scopes = claims.Scopes
	return p
}

// sessionCookie is the cookie the web app stores its JWT in (team-frontend
// SESSION_COOKIE). Kept here so beacons carrying only a cookie still attribute.
const sessionCookie = "session"

func principalType(t string) commonv1.PrincipalType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "user":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case "service":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	}
}
