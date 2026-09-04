// JWKS client (ADR-0006). The gateway holds no signing material: it fetches
// team-identity's RSA *public* keys from JWKS_URL, caches them by `kid`, and
// refreshes on a TTL or when it meets an unknown `kid` (one bounded forced
// refresh) so a rotated-in key becomes usable without a redeploy.
package token

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwk is a single RSA public key in JWK form (RFC 7517/7518).
type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

// keySet is a TTL-cached JWKS keyset fetched from a URL, indexed by kid.
type keySet struct {
	ttl       time.Duration
	minForced time.Duration // floor between forced refreshes on an unknown kid
	fetch     func(context.Context) (map[string]*rsa.PublicKey, error)
	now       func() time.Time

	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastFetch  time.Time
	lastForced time.Time
}

func newHTTPKeySet(url string, ttl time.Duration) *keySet {
	client := &http.Client{Timeout: 5 * time.Second}
	return &keySet{
		ttl:       ttl,
		minForced: 10 * time.Second,
		now:       time.Now,
		keys:      map[string]*rsa.PublicKey{},
		fetch: func(ctx context.Context) (map[string]*rsa.PublicKey, error) {
			return fetchJWKS(ctx, client, url)
		},
	}
}

// refresh fetches the JWKS and atomically swaps the cache. On error the previous
// cache is retained (fail-soft) and the error is returned for logging.
func (ks *keySet) refresh(ctx context.Context) error {
	keys, err := ks.fetch(ctx)
	if err != nil {
		return err
	}
	ks.mu.Lock()
	ks.keys = keys
	ks.lastFetch = ks.now()
	ks.mu.Unlock()
	return nil
}

func (ks *keySet) lookup(kid string) *rsa.PublicKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.keys[kid]
}

func (ks *keySet) stale() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.lastFetch.IsZero() || ks.now().Sub(ks.lastFetch) > ks.ttl
}

// keyForKID returns the cached public key for kid, refreshing when the cache is
// stale or when kid is unknown (a single rate-limited forced refresh) so a
// newly published key is honored.
func (ks *keySet) keyForKID(kid string) (*rsa.PublicKey, error) {
	if key := ks.lookup(kid); key != nil && !ks.stale() {
		return key, nil
	}

	// Decide whether a refresh is warranted: stale cache always; an unknown kid
	// only if we haven't just forced one (bounds hammering identity on bad tokens).
	ks.mu.Lock()
	doRefresh := ks.lastFetch.IsZero() || ks.now().Sub(ks.lastFetch) > ks.ttl
	if !doRefresh {
		if _, ok := ks.keys[kid]; !ok && ks.now().Sub(ks.lastForced) > ks.minForced {
			doRefresh = true
			ks.lastForced = ks.now()
		}
	}
	ks.mu.Unlock()

	if doRefresh {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = ks.refresh(ctx) // fail-soft: keep the prior cache on error
		cancel()
	}

	if key := ks.lookup(kid); key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("token: no JWKS key for kid %q", kid)
}

// fetchJWKS GETs the JWKS document and parses its RSA public keys.
func fetchJWKS(ctx context.Context, client *http.Client, url string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("token: build JWKS request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token: fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token: JWKS returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("token: read JWKS body: %w", err)
	}
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("token: decode JWKS: %w", err)
	}
	out := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := parseRSAJWK(k)
		if err != nil {
			continue // skip malformed key, keep the rest
		}
		out[k.Kid] = pub
	}
	return out, nil
}

func parseRSAJWK(k jwk) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	if len(nb) == 0 || len(eb) == 0 {
		return nil, fmt.Errorf("empty modulus/exponent")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: int(new(big.Int).SetBytes(eb).Int64()),
	}, nil
}
