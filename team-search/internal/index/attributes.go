package index

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Dynamic attribute facets (add-tag-classifier-filter-enrichment).
//
// The read-model carries two shapes of classified tags, both written by the
// indexer from team-ai's tag classifier:
//
//   - facet_tags: a flat keyword array of "group:slug" entries for the listing's
//     canonical SPU tags (e.g. "connectivity:bluetooth-5-3"). One keyword field
//     serves every facet group, so a newly promoted group or tag needs no mapping
//     change and a terms aggregation over it yields every group at once.
//   - skus: a NESTED array, one object per variant, each with its own "group:slug"
//     attrs and its own is_in_stock. Nested objects are what keeps "navy AND 512GB"
//     from matching a listing whose navy variant is 256GB and whose 512GB variant is
//     black (the cross-variant false positive a flat array would give).
//
// Callers filter with request filter keys "tag.<group>" and "sku.<group>"; the
// value is one tag slug, or several separated by commas (OR within a group,
// AND across groups).

const (
	// FilterTagPrefix is the request filter key prefix for an SPU-level tag group.
	FilterTagPrefix = "tag."
	// FilterSkuPrefix is the request filter key prefix for a variant-level group.
	FilterSkuPrefix = "sku."

	fieldFacetTags = "facet_tags"
	nestedSkus     = "skus"
	fieldSkuAttrs  = "skus.attrs"
	fieldSkuStock  = "skus.is_in_stock"

	// maxFacetValues bounds how many values one group filter may carry.
	maxFacetValues = 20
	// attrAggSize is how many "group:slug" buckets each attribute aggregation asks for.
	attrAggSize = 200
	// maxBucketsPerGroup caps the buckets returned for one facet group.
	maxBucketsPerGroup = 12
)

var (
	groupPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	slugPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// SkuDoc is one variant in the nested skus field.
type SkuDoc struct {
	VariantID string   `json:"variant_id"`
	SkuCode   string   `json:"sku_code,omitempty"`
	Name      string   `json:"name,omitempty"`
	Price     int64    `json:"price"`
	Stock     int32    `json:"stock"`
	InStock   bool     `json:"is_in_stock"`
	Attrs     []string `json:"attrs"`
}

// AttrKey renders the keyword stored for one tag of a facet group.
func AttrKey(group, slug string) string { return group + ":" + slug }

// AttributeFacet is one facet group (e.g. "color") with its value buckets
// (key = canonical tag slug).
type AttributeFacet struct {
	Group   string
	Buckets []FacetBucket
}

// ParseAttrFilterKey reports whether key is a "tag.<group>" or "sku.<group>"
// filter key, and which. ok is false for any other key; err is set when the key has
// the prefix but the group is not a well-formed facet group name.
func ParseAttrFilterKey(key string) (prefix, group string, ok bool, err error) {
	for _, p := range []string{FilterTagPrefix, FilterSkuPrefix} {
		if strings.HasPrefix(key, p) {
			g := strings.TrimPrefix(key, p)
			if !groupPattern.MatchString(g) {
				return p, "", true, fmt.Errorf("filter %q: facet group must match %s", key, groupPattern)
			}
			return p, g, true, nil
		}
	}
	return "", "", false, nil
}

// ParseAttrFilterValues splits a filter value into validated tag slugs.
func ParseAttrFilterValues(key, value string) ([]string, error) {
	parts := strings.Split(value, ",")
	if len(parts) > maxFacetValues {
		return nil, fmt.Errorf("filter %q: at most %d values", key, maxFacetValues)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !slugPattern.MatchString(p) {
			return nil, fmt.Errorf("filter %q: value %q must be a tag slug (%s)", key, p, slugPattern)
		}
		out = append(out, p)
	}
	return out, nil
}

// attrTermClause matches one group's values against a keyword field.
func attrTermClause(field, group string, slugs []string) map[string]any {
	if len(slugs) == 1 {
		return map[string]any{"term": map[string]any{field: AttrKey(group, slugs[0])}}
	}
	keys := make([]string, len(slugs))
	for i, s := range slugs {
		keys[i] = AttrKey(group, s)
	}
	return map[string]any{"terms": map[string]any{field: keys}}
}

// attrFilterClauses turns the tag.* / sku.* entries of filters into OpenSearch
// clauses: one term(s) clause per tag group on facet_tags, and ONE nested clause
// over skus holding every sku group plus is_in_stock, so all sku conditions must
// hold on the same variant. A malformed key or value yields match_none (the
// handler rejects those before they get here; this keeps the index safe alone).
// skuFilters is the same sku conditions (without the nested wrapper) for the
// facet aggregation; consumed reports which filter keys were handled.
func attrFilterClauses(filters map[string]string) (clauses []any, skuFilters []any, consumed map[string]bool) {
	consumed = map[string]bool{}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var skuClauses []any
	for _, k := range keys {
		prefix, group, ok, err := ParseAttrFilterKey(k)
		if !ok {
			continue
		}
		consumed[k] = true
		var slugs []string
		if err == nil {
			slugs, err = ParseAttrFilterValues(k, filters[k])
		}
		if err != nil {
			clauses = append(clauses, map[string]any{"match_none": map[string]any{}})
			continue
		}
		if prefix == FilterTagPrefix {
			clauses = append(clauses, attrTermClause(fieldFacetTags, group, slugs))
		} else {
			skuClauses = append(skuClauses, attrTermClause(fieldSkuAttrs, group, slugs))
		}
	}
	if len(skuClauses) > 0 {
		// Only in-stock variants count as a match: a buyer filtering on 512GB does
		// not want a listing whose 512GB variant is sold out.
		skuFilters = append([]any{map[string]any{"term": map[string]any{fieldSkuStock: true}}}, skuClauses...)
		clauses = append(clauses, map[string]any{"nested": map[string]any{
			"path":  nestedSkus,
			"query": map[string]any{"bool": map[string]any{"filter": skuFilters}},
		}})
	}
	return clauses, skuFilters, consumed
}

// attrAggs builds the dynamic-facet aggregations. The sku aggregation counts
// variants, then reverse_nested folds that back to LISTINGS, restricted to
// in-stock variants that satisfy every active sku filter, so a bucket count is
// "listings you would get by adding this value".
func attrAggs(filters map[string]string) map[string]any {
	_, skuFilters, _ := attrFilterClauses(filters)
	if len(skuFilters) == 0 {
		skuFilters = []any{map[string]any{"term": map[string]any{fieldSkuStock: true}}}
	}
	return map[string]any{
		"tag_facets": map[string]any{"terms": map[string]any{"field": fieldFacetTags, "size": attrAggSize}},
		"sku_facets": map[string]any{
			"nested": map[string]any{"path": nestedSkus},
			"aggs": map[string]any{"available": map[string]any{
				"filter": map[string]any{"bool": map[string]any{"filter": skuFilters}},
				"aggs": map[string]any{"attrs": map[string]any{
					"terms": map[string]any{"field": fieldSkuAttrs, "size": attrAggSize},
					"aggs":  map[string]any{"listings": map[string]any{"reverse_nested": map[string]any{}}},
				}},
			}},
		},
	}
}

type osSkuAgg struct {
	Available struct {
		Attrs struct {
			Buckets []struct {
				Key      any `json:"key"`
				Listings struct {
					DocCount int64 `json:"doc_count"`
				} `json:"listings"`
			} `json:"buckets"`
		} `json:"attrs"`
	} `json:"available"`
}

// groupBuckets folds "group:slug" bucket keys into per-group facets, keeping the
// order OpenSearch returned (count desc) for both the groups (by first
// appearance) and the buckets within a group, capped at maxBucketsPerGroup.
func groupBuckets(keys []string, counts []int64) []AttributeFacet {
	out := []AttributeFacet{}
	at := map[string]int{}
	for i, k := range keys {
		group, slug, found := strings.Cut(k, ":")
		if !found || group == "" || slug == "" || counts[i] <= 0 {
			continue
		}
		gi, seen := at[group]
		if !seen {
			gi = len(out)
			at[group] = gi
			out = append(out, AttributeFacet{Group: group, Buckets: []FacetBucket{}})
		}
		if len(out[gi].Buckets) < maxBucketsPerGroup {
			out[gi].Buckets = append(out[gi].Buckets, FacetBucket{Key: slug, Count: counts[i]})
		}
	}
	return out
}

func parseTagFacets(a osTermsAgg) []AttributeFacet {
	keys := make([]string, len(a.Buckets))
	counts := make([]int64, len(a.Buckets))
	for i, b := range a.Buckets {
		keys[i], counts[i] = termKey(b.Key), b.DocCount
	}
	return groupBuckets(keys, counts)
}

func parseSkuFacets(a osSkuAgg) []AttributeFacet {
	bs := a.Available.Attrs.Buckets
	keys := make([]string, len(bs))
	counts := make([]int64, len(bs))
	for i, b := range bs {
		keys[i], counts[i] = termKey(b.Key), b.Listings.DocCount
	}
	return groupBuckets(keys, counts)
}

// ValidAttr reports whether group and slug are well-formed, i.e. a facet value a
// tag.<group> / sku.<group> filter could ever name. The indexer drops anything
// else so a malformed classifier answer can never produce an unfilterable facet.
func ValidAttr(group, slug string) bool {
	return groupPattern.MatchString(group) && slugPattern.MatchString(slug)
}
