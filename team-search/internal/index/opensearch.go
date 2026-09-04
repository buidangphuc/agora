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

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
)

// ListingDoc is the indexed shape of a listing (the read-model document).
// Version is the monotonic read-model version (AD2), sourced from the event's
// occurred_at; it drives OpenSearch external versioning so out-of-order events
// cannot overwrite newer state.
type ListingDoc struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Currency    string  `json:"currency"`
	Price       int64   `json:"price"`
	CategoryID  string  `json:"category_id"`
	SellerID    string  `json:"seller_id"`
	Rating      float64 `json:"rating"`
	Version     int64   `json:"version"`
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
	Delete(ctx context.Context, id string) error
	Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (SearchResult, error)
	Suggest(ctx context.Context, prefix string, limit int) ([]string, error)
}

// OpenSearchIndex implements Index against an OpenSearch cluster.
type OpenSearchIndex struct {
	client *opensearch.Client
	name   string
}

// New builds an OpenSearch-backed index for the given URL + index name.
func New(url, name string) (*OpenSearchIndex, error) {
	client, err := opensearch.NewClient(opensearch.Config{Addresses: []string{url}})
	if err != nil {
		return nil, fmt.Errorf("opensearch client: %w", err)
	}
	return &OpenSearchIndex{client: client, name: name}, nil
}

// indexMapping: title as search_as_you_type powers both full-text and prefix
// suggestions; status/currency/category_id are keyword filters; price is numeric.
const indexMapping = `{
  "settings": {
    "analysis": {
      "analyzer": {
        "default": { "type": "standard" }
      }
    }
  },
  "mappings": {
    "properties": {
      "id":          { "type": "keyword" },
      "title":       { "type": "search_as_you_type" },
      "description": { "type": "text" },
      "status":      { "type": "keyword" },
      "currency":    { "type": "keyword" },
      "price":       { "type": "long" },
      "category_id": { "type": "keyword" },
      "seller_id":   { "type": "keyword" },
      "rating":      { "type": "float" },
      "version":     { "type": "long" }
    }
  }
}`

// EnsureIndex creates the listings index with indexMapping if it doesn't exist.
func (o *OpenSearchIndex) EnsureIndex(ctx context.Context) error {
	res, err := opensearchapi.IndicesExistsRequest{
		Index: []string{o.name},
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("check index %q: %w", o.name, err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		return nil
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
			return nil
		}
		return fmt.Errorf("create index %q: %s", o.name, msg)
	}
	return nil
}

// Upsert adds or replaces a whole document by id (idempotent). When the doc
// carries a positive Version it is applied with OpenSearch external versioning
// (AD2): OpenSearch rejects an incoming version <= the stored one with 409, which
// we treat as a no-op so a re-delivered or out-of-order event never overwrites
// newer state.
func (o *OpenSearchIndex) Upsert(ctx context.Context, doc ListingDoc) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req := opensearchapi.IndexRequest{
		Index:      o.name,
		DocumentID: doc.ID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}
	if doc.Version > 0 {
		v := int(doc.Version)
		req.Version = &v
		req.VersionType = "external"
	}
	res, err := req.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("index doc %q: %w", doc.ID, err)
	}
	defer res.Body.Close()
	if res.StatusCode == 409 {
		return nil // stale/duplicate version rejected by the version guard.
	}
	if res.IsError() {
		return fmt.Errorf("index doc %q: %s", doc.ID, res.String())
	}
	return nil
}

// versionGuardScript applies the partial fields only when the incoming version is
// newer than the stored one (AD2). With no `upsert`/`scripted_upsert` clause, an
// update to a missing (tombstoned) document returns 404 and is a no-op, so a
// stale partial update never resurrects a deleted listing.
const versionGuardScript = `if (params.version > 0 && ctx._source.version != null && ctx._source.version >= params.version) { ctx.op = 'noop'; } else { for (entry in params.fields.entrySet()) { ctx._source[entry.getKey()] = entry.getValue(); } if (params.version > 0) { ctx._source.version = params.version; } }`

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
		return nil // tombstoned/absent listing: do not resurrect.
	}
	if res.IsError() {
		return fmt.Errorf("partial update doc %q: %s", id, res.String())
	}
	return nil
}

// Delete removes a listing from the index (ignores a missing document).
func (o *OpenSearchIndex) Delete(ctx context.Context, id string) error {
	res, err := opensearchapi.DeleteRequest{
		Index:      o.name,
		DocumentID: id,
		Refresh:    "true",
	}.Do(ctx, o.client)
	if err != nil {
		return fmt.Errorf("delete doc %q: %w", id, err)
	}
	defer res.Body.Close()
	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("delete doc %q: %s", id, res.String())
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
	filterClauses := make([]any, 0, len(filters)+2)
	for k, v := range filters {
		filterClauses = append(filterClauses, map[string]any{"term": map[string]any{k: v}})
	}
	if categoryID != "" {
		filterClauses = append(filterClauses, map[string]any{"term": map[string]any{"category_id": categoryID}})
	}
	if minPrice > 0 || maxPrice > 0 {
		rangeQ := map[string]any{}
		if minPrice > 0 {
			rangeQ["gte"] = minPrice
		}
		if maxPrice > 0 {
			rangeQ["lte"] = maxPrice
		}
		filterClauses = append(filterClauses, map[string]any{"range": map[string]any{"price": rangeQ}})
	}
	if minRating > 0 {
		filterClauses = append(filterClauses, map[string]any{"range": map[string]any{"rating": map[string]any{"gte": minRating}}})
	}

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

	// Sorting
	switch sortBy {
	case searchv1.SortBy_SORT_BY_PRICE_ASC:
		body["sort"] = []any{map[string]any{"price": map[string]any{"order": "asc"}}}
	case searchv1.SortBy_SORT_BY_PRICE_DESC:
		body["sort"] = []any{map[string]any{"price": map[string]any{"order": "desc"}}}
	case searchv1.SortBy_SORT_BY_NEWEST:
		body["sort"] = []any{map[string]any{"_id": map[string]any{"order": "desc"}}}
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
		"query": map[string]any{
			"multi_match": map[string]any{
				"query":  prefix,
				"type":   "bool_prefix",
				"fields": []string{"title", "title._2gram", "title._3gram"},
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
