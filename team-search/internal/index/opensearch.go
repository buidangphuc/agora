// Package index is team-search's read-model store: an OpenSearch index of
// listings, kept up to date by the Kafka indexer and queried by the gRPC search
// API. The engine is behind the Index interface so it can grow to hybrid/vector
// later (ADR-0005) and so handlers/consumers can be tested with a fake.
package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
)

// ListingDoc is the indexed shape of a listing (the read-model document).
// Version is the monotonic read-model version (AD2), sourced from the event's
// occurred_at; the write guard (writeScript) compares it with the stored one so
// out-of-order events cannot overwrite newer state. Rating is not written: no
// listing event carries one (D9); the mapping keeps the field.
type ListingDoc struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Currency    string `json:"currency"`
	Price       int64  `json:"price"`
	CategoryID  string `json:"category_id"`
	SellerID    string `json:"seller_id"`
	Version     int64  `json:"version"`
	// Stock is Listing.stock from the event; nil when the doc holds none. It is
	// written under its own stock_version guard (D2), never with the base fields.
	Stock *int32 `json:"stock,omitempty"`
	// CreatedAt is the CREATED event's occurred_at in epoch millis (D7); nil on
	// every other event. The script keeps the earliest one ever seen.
	CreatedAt     *int64    `json:"created_at,omitempty"`
	Embedding     []float32 `json:"embedding,omitempty"`
	VectorPending bool      `json:"vector_pending,omitempty"`
}

// Hit is one search result.
type Hit struct {
	ListingID string
	Score     float64
}

// FacetBucket is one facet value and the number of matching listings that carry
// it (key = category_id / seller_id / price-range label / rating floor).
type FacetBucket struct {
	Key   string
	Count int64
}

// Facets are the aggregation counts computed over the SAME filtered set as the
// hits, so the UI can offer filter navigation. Every slice is non-nil (possibly
// empty) so callers never nil-panic on an empty result.
type Facets struct {
	Categories  []FacetBucket
	PriceRanges []FacetBucket
	Ratings     []FacetBucket
	Sellers     []FacetBucket
}

// SearchResult is a page of hits plus the total match count and facet counts.
type SearchResult struct {
	Hits   []Hit
	Total  int64
	Facets Facets
}

// Index is the read-model port the handler and consumer depend on.
type Index interface {
	EnsureIndex(ctx context.Context) error
	Upsert(ctx context.Context, doc ListingDoc) error
	PartialUpdate(ctx context.Context, id string, partialDoc map[string]interface{}) error
	// UpdateStock applies a ListingStockChanged under the stock_version guard (D2);
	// it never creates a document and is a no-op on a tombstone.
	UpdateStock(ctx context.Context, id string, stock int32, version int64) error
	// Delete writes a versioned tombstone (D5); version is the delete event's occurred_at in ns.
	Delete(ctx context.Context, id string, version int64) error
	Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (SearchResult, error)
	SearchVector(ctx context.Context, vector []float32, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (SearchResult, error)
	Suggest(ctx context.Context, prefix string, limit int) ([]string, error)
}

// OpenSearchIndex implements Index against an OpenSearch cluster.
type OpenSearchIndex struct {
	client *opensearch.Client
	name   string
	now    func() time.Time
}

// New builds an OpenSearch-backed index for the given URL + index name.
func New(url, name string) (*OpenSearchIndex, error) {
	client, err := opensearch.NewClient(opensearch.Config{Addresses: []string{url}})
	if err != nil {
		return nil, fmt.Errorf("opensearch client: %w", err)
	}
	return &OpenSearchIndex{client: client, name: name, now: time.Now}, nil
}

// indexMapping: title as search_as_you_type powers both full-text and prefix
// suggestions; status/currency/category_id are keyword filters; price is numeric;
// embedding is a dense vector supporting Lucene k-NN similarity (ADR-0005).
const indexMapping = `{
  "settings": {
    "index": {
      "knn": true
    },
    "analysis": {
      "analyzer": {
        "default": { "type": "standard" }
      }
    }
  },
  "mappings": {
    "properties": {
      "id":             { "type": "keyword" },
      "title":          { "type": "search_as_you_type" },
      "description":    { "type": "text" },
      "status":         { "type": "keyword" },
      "currency":       { "type": "keyword" },
      "price":          { "type": "long" },
      "category_id":    { "type": "keyword" },
      "seller_id":      { "type": "keyword" },
      "rating":         { "type": "float" },
      "version":        { "type": "long" },
      "stock":          { "type": "integer" },
      "stock_version":  { "type": "long" },
      "created_at":     { "type": "date", "format": "epoch_millis" },
      "tombstoned_at":  { "type": "date", "format": "epoch_millis" },
      "embedding":      {
        "type": "knn_vector",
        "dimension": 384,
        "method": {
          "name": "hnsw",
          "engine": "lucene",
          "space_type": "cosinesimil"
        }
      },
      "vector_pending": { "type": "boolean" }
    }
  }
}`

// additiveMapping is the put-mapping body for the fields added after the index
// was first created (D10). It must stay identical to their entries in
// indexMapping: an existing index gains them, and a second put (or a concurrent
// one from the other process at boot) is a no-op. An existing field of another
// type makes the put fail, so a boot never runs on a mapping it cannot use.
const additiveMapping = `{
  "properties": {
    "stock":         { "type": "integer" },
    "stock_version": { "type": "long" },
    "created_at":    { "type": "date", "format": "epoch_millis" },
    "tombstoned_at": { "type": "date", "format": "epoch_millis" }
  }
}`

// EnsureIndex creates the listings index with indexMapping if it doesn't exist,
// and otherwise idempotently puts the additive fields onto the existing mapping.
func (o *OpenSearchIndex) EnsureIndex(ctx context.Context) error {
	res, err := opensearchapi.IndicesExistsRequest{
		Index: []string{o.name},
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("check index %q: %w", o.name, err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		return o.ensureAdditiveMapping(ctx)
	}

	createRes, err := opensearchapi.IndicesCreateRequest{
		Index: o.name,
		Body:  strings.NewReader(indexMapping),
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("create index %q: %w", o.name, err)
	}
	defer createRes.Body.Close()
	if createRes.IsError() {
		// Idempotent: a concurrent creator (the query server and the indexer race
		// on cold boot) may have created the index between our exists-check and
		// create. OpenSearch answers the loser with 400 resource_already_exists_exception
		// — treat that as success rather than crashing the process.
		msg := createRes.String()
		if createRes.StatusCode == 400 && strings.Contains(msg, "resource_already_exists_exception") {
			return o.ensureAdditiveMapping(ctx)
		}
		return fmt.Errorf("create index %q: %s", o.name, msg)
	}
	return nil
}

// ensureAdditiveMapping issues the idempotent PUT _mapping for additiveMapping.
func (o *OpenSearchIndex) ensureAdditiveMapping(ctx context.Context) error {
	res, err := opensearchapi.IndicesPutMappingRequest{
		Index: []string{o.name},
		Body:  strings.NewReader(additiveMapping),
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("put mapping on %q: %w", o.name, err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("put mapping on %q (stock, stock_version, created_at, tombstoned_at): %s", o.name, res.String())
	}
	return nil
}

// statusDeleted marks a tombstone document (D5). It is never published, so the
// published-only default hides it, and the handler rejects it as a status filter.
const statusDeleted = "deleted"

// writeScript is the one write guard for every whole-document write (D1, D5).
// It runs as a scripted_upsert, so it sees an absent document as an empty
// _source with ctx.op == 'create', and decides in one atomic request:
//
//   - kind 'upsert' on a live document: the base fields are replaced by
//     params.doc unless the incoming version is positive and not newer than
//     the stored one. Stock has its own guard (D2): params.stock is taken with
//     stock_version = version only when that is newer than the stored
//     stock_version (even if the base fields are stale), otherwise the stored
//     stock and stock_version are carried forward. created_at (D7) is
//     set-if-absent-or-earlier from params.created_at (a CREATED event's
//     occurred_at, also when its base fields are stale) and otherwise carried
//     forward; it is never written onto a tombstone.
//   - kind 'upsert' on a tombstone: noop when the incoming version is 0 (no
//     occurred_at) or not newer than the tombstone's; a newer one replaces it.
//   - kind 'delete': writes {id, status: deleted, version, tombstoned_at} and
//     drops every other field, unless the stored document (live or tombstone)
//     is already at or past the delete's version; an unversioned delete keeps
//     the stored version so it cannot lower the guard.
const writeScript = `
Map s = ctx._source;
boolean absent = ctx.op == 'create' || s.isEmpty();
boolean tomb = !absent && 'deleted'.equals(s.status);
long v = ((Number) params.version).longValue();
boolean hasCur = !absent && s.version != null;
long cur = hasCur ? ((Number) s.version).longValue() : 0L;
if (params.kind == 'delete') {
  boolean stale = tomb ? (hasCur && cur >= v) : (v > 0 && hasCur && cur >= v);
  if (stale) {
    ctx.op = 'noop';
  } else {
    long nv = v > 0 ? v : cur;
    s.clear();
    s.id = params.id;
    s.status = 'deleted';
    s.version = nv;
    s.tombstoned_at = params.now;
  }
} else {
  boolean stale = tomb ? (v <= 0 || (hasCur && cur >= v)) : (v > 0 && hasCur && cur >= v);
  boolean live = !(tomb && stale);
  def oldStock = (absent || tomb) ? null : s.stock;
  def oldSV = (absent || tomb) ? null : s.stock_version;
  def oldCreated = (absent || tomb) ? null : s.created_at;
  boolean takeStock = live && params.stock != null && (oldSV == null || ((Number) oldSV).longValue() < v);
  def created = oldCreated;
  if (live && params.created_at != null && (created == null || ((Number) created).longValue() > ((Number) params.created_at).longValue())) {
    created = params.created_at;
  }
  boolean newCreated = live && created != null && (oldCreated == null || ((Number) created).longValue() != ((Number) oldCreated).longValue());
  if (!stale) {
    s.clear();
    s.putAll(params.doc);
  }
  if (takeStock) {
    s.stock = params.stock;
    s.stock_version = v;
  } else if (!stale && oldSV != null) {
    s.stock = oldStock;
    s.stock_version = oldSV;
  }
  if (live && created != null) {
    s.created_at = created;
  }
  if (stale && !takeStock && !newCreated) {
    ctx.op = 'noop';
  }
}
`

// Upsert adds or replaces a whole document by id through writeScript, so a
// re-delivered, out-of-order or unversioned event never overwrites newer state
// and never revives a tombstone it does not postdate.
func (o *OpenSearchIndex) Upsert(ctx context.Context, doc ListingDoc) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	// Guarded separately by the script: stock (D2) and created_at (D7).
	delete(fields, "stock")
	delete(fields, "created_at")
	var stock, createdAt any
	if doc.Stock != nil {
		stock = *doc.Stock
	}
	if doc.CreatedAt != nil {
		createdAt = *doc.CreatedAt
	}
	return o.write(ctx, doc.ID, map[string]any{
		"kind":       "upsert",
		"version":    doc.Version,
		"doc":        fields,
		"stock":      stock,
		"created_at": createdAt,
	})
}

// Delete writes the tombstone for id through writeScript (D5): the delete
// event's version is recorded so an older event cannot resurrect the listing,
// and tombstoned_at (the indexer's clock) drives the purge (D6).
func (o *OpenSearchIndex) Delete(ctx context.Context, id string, version int64) error {
	return o.write(ctx, id, map[string]any{
		"kind":    "delete",
		"id":      id,
		"version": version,
		"now":     o.now().UnixMilli(),
	})
}

func (o *OpenSearchIndex) write(ctx context.Context, id string, params map[string]any) error {
	body, err := json.Marshal(map[string]any{
		"scripted_upsert": true,
		"upsert":          map[string]any{},
		"script": map[string]any{
			"lang":   "painless",
			"source": writeScript,
			"params": params,
		},
	})
	if err != nil {
		return err
	}
	res, err := opensearchapi.UpdateRequest{
		Index:      o.name,
		DocumentID: id,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("%s doc %q: %w", params["kind"], id, err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("%s doc %q: %s", params["kind"], id, res.String())
	}
	return nil
}

// stockScript applies a ListingStockChanged (D2): a noop on a tombstone or when
// the stored stock_version is at or past the event's; otherwise it sets stock and
// stock_version and leaves version (the base-field guard) alone. It runs without
// an upsert clause, so a missing document is a 404 and nothing is created.
const stockScript = `if ('deleted'.equals(ctx._source.status)) { ctx.op = 'noop'; } else if (ctx._source.stock_version != null && ((Number) ctx._source.stock_version).longValue() >= params.sv) { ctx.op = 'noop'; } else { ctx._source.stock = params.stock; ctx._source.stock_version = params.sv; }`

// UpdateStock projects a stock change onto an existing document under the
// stock_version guard. An absent document (404) is acknowledged as a no-op.
func (o *OpenSearchIndex) UpdateStock(ctx context.Context, id string, stock int32, version int64) error {
	body, err := json.Marshal(map[string]any{
		"script": map[string]any{
			"lang":   "painless",
			"source": stockScript,
			"params": map[string]any{"stock": stock, "sv": version},
		},
	})
	if err != nil {
		return err
	}
	res, err := opensearchapi.UpdateRequest{
		Index:      o.name,
		DocumentID: id,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("update stock %q: %w", id, err)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil // unknown listing: a stock event never creates a document.
	}
	if res.IsError() {
		return fmt.Errorf("update stock %q: %s", id, res.String())
	}
	return nil
}

// versionGuardScript applies the partial fields only when the incoming version is
// newer than the stored one (AD2), and never on a tombstone, whatever the version
// (D5). With no `upsert`/`scripted_upsert` clause, an update to a missing
// document returns 404 and is a no-op, so a partial update never creates one.
const versionGuardScript = `if ('deleted'.equals(ctx._source.status)) { ctx.op = 'noop'; } else if (params.version > 0 && ctx._source.version != null && ctx._source.version >= params.version) { ctx.op = 'noop'; } else { for (entry in params.fields.entrySet()) { ctx._source[entry.getKey()] = entry.getValue(); } if (params.version > 0) { ctx._source.version = params.version; } }`

// PartialUpdate updates only specific fields of a document without re-indexing
// full text. The version guard (AD2) is enforced by a scripted update: stale
// versions are dropped and a missing document is NOT recreated.
func (o *OpenSearchIndex) PartialUpdate(ctx context.Context, id string, partialDoc map[string]interface{}) error {
	version, _ := partialDoc["version"].(int64)
	fields := make(map[string]interface{}, len(partialDoc))
	for k, v := range partialDoc {
		if k == "version" {
			continue
		}
		fields[k] = v
	}
	updatePayload := map[string]interface{}{
		"script": map[string]interface{}{
			"lang":   "painless",
			"source": versionGuardScript,
			"params": map[string]interface{}{
				"version": version,
				"fields":  fields,
			},
		},
	}
	body, err := json.Marshal(updatePayload)
	if err != nil {
		return err
	}
	res, err := opensearchapi.UpdateRequest{
		Index:      o.name,
		DocumentID: id,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("partial update doc %q: %w", id, err)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil // absent listing: do not create.
	}
	if res.IsError() {
		return fmt.Errorf("partial update doc %q: %s", id, res.String())
	}
	return nil
}

type osSearchResponse struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID     string     `json:"_id"`
			Score  float64    `json:"_score"`
			Source ListingDoc `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations osAggregations `json:"aggregations"`
}

// osAggregations mirrors the aggregation block OpenSearch returns for the four
// facets. terms aggs yield a buckets ARRAY; the keyed range/filters aggs yield a
// buckets OBJECT (key -> {doc_count}), so those are decoded as maps.
type osAggregations struct {
	Categories  osTermsAgg `json:"categories"`
	Sellers     osTermsAgg `json:"sellers"`
	PriceRanges osKeyedAgg `json:"price_ranges"`
	Ratings     osKeyedAgg `json:"ratings"`
}

type osTermsAgg struct {
	Buckets []struct {
		Key      any   `json:"key"`
		DocCount int64 `json:"doc_count"`
	} `json:"buckets"`
}

type osKeyedAgg struct {
	Buckets map[string]struct {
		DocCount int64 `json:"doc_count"`
	} `json:"buckets"`
}

// priceRangeBucket is a fixed price facet bucket. from/to are the OpenSearch
// range bounds (0 == unbounded on that side); label is the emitted FacetBucket
// key and drives a stable output order.
type priceRangeBucket struct {
	label string
	from  int64
	to    int64
}

// priceRangeBuckets are the fixed price-range facet buckets (F2): 0-100k,
// 100k-500k, 500k-1M, 1M+. Order here is the emitted facet order.
var priceRangeBuckets = []priceRangeBucket{
	{label: "0-100000", from: 0, to: 100000},
	{label: "100000-500000", from: 100000, to: 500000},
	{label: "500000-1000000", from: 500000, to: 1000000},
	{label: "1000000+", from: 1000000, to: 0},
}

// ratingBuckets are the cumulative rating facet floors (>=4, >=3, >=2, >=1).
// key is the emitted FacetBucket key ("4" == 4-plus stars); floor is the range
// gte bound. Modeled as a filters agg so buckets overlap (>=4 counts into >=3).
var ratingBuckets = []struct {
	key   string
	floor float64
}{
	{key: "4", floor: 4},
	{key: "3", floor: 3},
	{key: "2", floor: 2},
	{key: "1", floor: 1},
}

// facetAggs builds the aggregation block requested alongside every SearchListings
// query. It aggregates over the post-filter matched set (aggs sit outside the
// query in the request body but count only documents the query matched).
func facetAggs() map[string]any {
	priceRanges := make([]map[string]any, 0, len(priceRangeBuckets))
	for _, b := range priceRangeBuckets {
		r := map[string]any{"key": b.label}
		if b.from > 0 {
			r["from"] = b.from
		}
		if b.to > 0 {
			r["to"] = b.to
		}
		priceRanges = append(priceRanges, r)
	}
	ratingFilters := make(map[string]any, len(ratingBuckets))
	for _, b := range ratingBuckets {
		ratingFilters[b.key] = map[string]any{"range": map[string]any{"rating": map[string]any{"gte": b.floor}}}
	}
	return map[string]any{
		"categories": map[string]any{"terms": map[string]any{"field": "category_id", "size": 50}},
		"sellers":    map[string]any{"terms": map[string]any{"field": "seller_id", "size": 50}},
		"price_ranges": map[string]any{"range": map[string]any{
			"field":  "price",
			"keyed":  true,
			"ranges": priceRanges,
		}},
		"ratings": map[string]any{"filters": map[string]any{"filters": ratingFilters}},
	}
}

// parseFacets turns the raw aggregation block into Facets. Every slice is
// initialized (never nil), so an empty result set yields empty — not nil —
// facet buckets. Terms buckets are emitted in the order OpenSearch returns them
// (by count desc); the keyed price/rating buckets are emitted in the fixed order
// defined above so the UI order is stable regardless of JSON map iteration.
func parseFacets(aggs osAggregations) Facets {
	f := Facets{
		Categories:  make([]FacetBucket, 0, len(aggs.Categories.Buckets)),
		Sellers:     make([]FacetBucket, 0, len(aggs.Sellers.Buckets)),
		PriceRanges: make([]FacetBucket, 0, len(priceRangeBuckets)),
		Ratings:     make([]FacetBucket, 0, len(ratingBuckets)),
	}
	for _, b := range aggs.Categories.Buckets {
		f.Categories = append(f.Categories, FacetBucket{Key: termKey(b.Key), Count: b.DocCount})
	}
	for _, b := range aggs.Sellers.Buckets {
		f.Sellers = append(f.Sellers, FacetBucket{Key: termKey(b.Key), Count: b.DocCount})
	}
	for _, b := range priceRangeBuckets {
		f.PriceRanges = append(f.PriceRanges, FacetBucket{Key: b.label, Count: aggs.PriceRanges.Buckets[b.label].DocCount})
	}
	for _, b := range ratingBuckets {
		f.Ratings = append(f.Ratings, FacetBucket{Key: b.key, Count: aggs.Ratings.Buckets[b.key].DocCount})
	}
	return f
}

// termKey renders a terms-aggregation key (a keyword is a string; a numeric
// field comes back as a JSON number) as the string facet key.
func termKey(k any) string {
	switch v := k.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Listing status filter key and the only status served when the caller names none.
const (
	filterStatus    = "status"
	statusPublished = "published"
)

func statusClause(v string) map[string]any {
	return map[string]any{"term": map[string]any{filterStatus: v}}
}

// buildFilterClauses is the single place structured filters become OpenSearch
// filter clauses, shared by every retrieval strategy so hits, totals and facets
// see the same set.
//
// Visibility default: unless the filter set already carries a "status", a term
// status=published clause is added, so a draft, rejected or unspecified document
// is never matched by a caller that forgot the filter. A given status is used as
// is; whether the caller may ask for a non-published status is the handler's
// decision (handler.effectiveFilters), not the index's.
func buildFilterClauses(filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32) []any {
	clauses := make([]any, 0, len(filters)+4)
	if _, has := filters[filterStatus]; !has {
		clauses = append(clauses, statusClause(statusPublished))
	}
	for k, v := range filters {
		clauses = append(clauses, map[string]any{"term": map[string]any{k: v}})
	}
	if categoryID != "" {
		clauses = append(clauses, map[string]any{"term": map[string]any{"category_id": categoryID}})
	}
	if minPrice > 0 || maxPrice > 0 {
		rangeQ := map[string]any{}
		if minPrice > 0 {
			rangeQ["gte"] = minPrice
		}
		if maxPrice > 0 {
			rangeQ["lte"] = maxPrice
		}
		clauses = append(clauses, map[string]any{"range": map[string]any{"price": rangeQ}})
	}
	if minRating > 0 {
		clauses = append(clauses, map[string]any{"range": map[string]any{"rating": map[string]any{"gte": minRating}}})
	}
	return clauses
}

// sortClause is the key sort shared by both retrieval legs; nil means relevance.
// SORT_BY_NEWEST orders by creation time (D7), most recent first, listings with
// no recorded creation time last, ties by the id keyword (never _id).
func sortClause(sortBy searchv1.SortBy) []any {
	switch sortBy {
	case searchv1.SortBy_SORT_BY_PRICE_ASC:
		return []any{map[string]any{"price": map[string]any{"order": "asc"}}}
	case searchv1.SortBy_SORT_BY_PRICE_DESC:
		return []any{map[string]any{"price": map[string]any{"order": "desc"}}}
	case searchv1.SortBy_SORT_BY_NEWEST:
		return []any{
			map[string]any{"created_at": map[string]any{"order": "desc", "missing": "_last", "unmapped_type": "date"}},
			map[string]any{"id": map[string]any{"order": "asc"}},
		}
	}
	return nil
}

// Search runs a free-text (multi_match over title^2 + description) query with
// structured filters (category, price range, terms) and sorting, paginated by from/size.
func (o *OpenSearchIndex) Search(
	ctx context.Context,
	query string,
	filters map[string]string,
	categoryID string,
	minPrice, maxPrice int64,
	minRating int32,
	sortBy searchv1.SortBy,
	from, size int,
) (SearchResult, error) {
	must := map[string]any{"match_all": map[string]any{}}
	if strings.TrimSpace(query) != "" {
		must = map[string]any{
			"multi_match": map[string]any{
				"query":  query,
				"fields": []string{"title^2", "description"},
			},
		}
	}
	filterClauses := buildFilterClauses(filters, categoryID, minPrice, maxPrice, minRating)

	body := map[string]any{
		"from": from,
		"size": size,
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filterClauses,
			},
		},
		// Facet aggregations over the SAME filtered set as the hits (F2).
		"aggs": facetAggs(),
	}

	if sort := sortClause(sortBy); sort != nil {
		body["sort"] = sort
	}

	var parsed osSearchResponse
	if err := o.doSearch(ctx, body, &parsed); err != nil {
		return SearchResult{}, err
	}
	hits := make([]Hit, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		hits = append(hits, Hit{ListingID: h.Source.ID, Score: h.Score})
	}
	return SearchResult{
		Hits:   hits,
		Total:  parsed.Hits.Total.Value,
		Facets: parseFacets(parsed.Aggregations),
	}, nil
}

// SearchVector runs dense k-NN vector search with structured filters.
func (o *OpenSearchIndex) SearchVector(
	ctx context.Context,
	vector []float32,
	filters map[string]string,
	categoryID string,
	minPrice, maxPrice int64,
	minRating int32,
	sortBy searchv1.SortBy,
	from, size int,
) (SearchResult, error) {
	if len(vector) == 0 {
		return SearchResult{Facets: parseFacets(osAggregations{})}, nil
	}

	filterClauses := buildFilterClauses(filters, categoryID, minPrice, maxPrice, minRating)

	k := from + size
	if k <= 0 {
		k = 10
	}

	knnClause := map[string]any{
		"vector": vector,
		"k":      k,
	}
	if len(filterClauses) > 0 {
		knnClause["filter"] = map[string]any{
			"bool": map[string]any{
				"filter": filterClauses,
			},
		}
	}

	body := map[string]any{
		"from": from,
		"size": size,
		"query": map[string]any{
			"knn": map[string]any{
				"embedding": knnClause,
			},
		},
		"aggs": facetAggs(),
	}

	if sort := sortClause(sortBy); sort != nil {
		body["sort"] = sort
	}

	var parsed osSearchResponse
	if err := o.doSearch(ctx, body, &parsed); err != nil {
		return SearchResult{}, err
	}
	hits := make([]Hit, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		hits = append(hits, Hit{ListingID: h.Source.ID, Score: h.Score})
	}
	return SearchResult{
		Hits:   hits,
		Total:  parsed.Hits.Total.Value,
		Facets: parseFacets(parsed.Aggregations),
	}, nil
}

// Suggest returns type-ahead completions of listing titles for a prefix, using
// the search_as_you_type subfields (bool_prefix).
func (o *OpenSearchIndex) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	if strings.TrimSpace(prefix) == "" {
		return []string{}, nil
	}
	body := map[string]any{
		"size":    limit,
		"_source": []string{"title"},
		// Suggestions are public: only published titles, never a draft's or rejected one's.
		"query": map[string]any{
			"bool": map[string]any{
				"must": map[string]any{
					"multi_match": map[string]any{
						"query":  prefix,
						"type":   "bool_prefix",
						"fields": []string{"title", "title._2gram", "title._3gram"},
					},
				},
				"filter": []any{statusClause(statusPublished)},
			},
		},
	}
	var parsed osSearchResponse
	if err := o.doSearch(ctx, body, &parsed); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(parsed.Hits.Hits))
	out := make([]string, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		if _, dup := seen[h.Source.Title]; dup || h.Source.Title == "" {
			continue
		}
		seen[h.Source.Title] = struct{}{}
		out = append(out, h.Source.Title)
	}
	return out, nil
}

func (o *OpenSearchIndex) doSearch(ctx context.Context, body map[string]any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res, err := opensearchapi.SearchRequest{
		Index: []string{o.name},
		Body:  bytes.NewReader(raw),
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("search: %s", res.String())
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// compile-time assertion.
var _ Index = (*OpenSearchIndex)(nil)
