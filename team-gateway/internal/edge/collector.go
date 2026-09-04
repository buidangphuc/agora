package edge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/events"
)

// maxBeaconBytes caps the request body a browser beacon may send. Telemetry is
// small; anything larger is a malformed/abusive request.
const maxBeaconBytes = 64 * 1024

// trackBeacon is the browser beacon shape (see team-frontend/src/lib/track.ts).
// It carries behavioral context ONLY — never authenticated identity, which the
// edge attaches via the envelope principal (contract forbids PII in payload).
type trackBeacon struct {
	Type        string            `json:"type"`
	ListingID   string            `json:"listingId"`
	SessionID   string            `json:"sessionId"`
	AnonymousID string            `json:"anonymousId"`
	Path        string            `json:"path"`
	Referrer    string            `json:"referrer"`
	Position    uint32            `json:"position"`
	Query       string            `json:"query"`
	Properties  map[string]string `json:"properties"`
}

// beaconEventTypes maps the beacon's lowercase action name to its EventType.
// An unknown/empty type is rejected (nothing is produced).
var beaconEventTypes = map[string]analyticsv1.EventType{
	"view":        analyticsv1.EventType_EVENT_TYPE_VIEW,
	"click":       analyticsv1.EventType_EVENT_TYPE_CLICK,
	"add_to_cart": analyticsv1.EventType_EVENT_TYPE_ADD_TO_CART,
	"impression":  analyticsv1.EventType_EVENT_TYPE_IMPRESSION,
}

// HandleTrack builds the pure edge-telemetry collector: parse the beacon (single
// or a small batch), reject a malformed/unknown-type body with a 4xx, map each
// beacon to a TrackingEvent, stamp the edge-resolved principal on the envelope,
// and forward to Kafka. It holds no business logic and owns no analytics storage
// (Rule 2). Delivery is best-effort: a produce error is logged, the response is
// 204 so a dropped beacon never surfaces to the user.
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

		// Validate every beacon BEFORE producing anything, so a malformed batch
		// rejects atomically without emitting a partial set.
		evs := make([]*analyticsv1.TrackingEvent, 0, len(beacons))
		for _, b := range beacons {
			et, ok := beaconEventTypes[strings.ToLower(strings.TrimSpace(b.Type))]
			if !ok {
				http.Error(w, "unknown event type", http.StatusBadRequest)
				return
			}
			evs = append(evs, &analyticsv1.TrackingEvent{
				EventType:   et,
				ListingId:   b.ListingID,
				SessionId:   b.SessionID,
				AnonymousId: b.AnonymousID,
				PagePath:    b.Path,
				Referrer:    b.Referrer,
				Position:    b.Position,
				SearchQuery: b.Query,
				Properties:  b.Properties,
			})
		}

		principal := e.beaconPrincipal(r)
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = newRequestID()
		}

		for _, ev := range evs {
			if err := pub.PublishTrackingEvent(r.Context(), ev, principal, requestID); err != nil {
				// Best-effort: log and keep going; the browsing action must not fail.
				logger.Warn("publish tracking event",
					slog.String("event_type", ev.GetEventType().String()),
					slog.String("listing_id", ev.GetListingId()),
					slog.Any("err", err),
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
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

	claims, err := e.verifier.Verify(tok)
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
