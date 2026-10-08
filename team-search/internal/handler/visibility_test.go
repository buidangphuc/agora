package handler

import (
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
)

func principal(id string, t commonv1.PrincipalType) *commonv1.Principal {
	return &commonv1.Principal{Id: id, Type: t}
}

// Table over every scenario in specs/search-query-correctness ("Only a listing's
// owner...") and specs/saved-searches ("Every saved-search operation...").
func TestEffectiveFilters(t *testing.T) {
	user := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	anon := commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	svc := commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	unspec := commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED

	cases := []struct {
		name    string
		p       *commonv1.Principal
		filters map[string]string
		want    codes.Code // codes.OK = accepted
	}{
		{"no filters", principal("a", user), nil, codes.OK},
		{"no status, anonymous", principal("anonymous", anon), map[string]string{"category_id": "c"}, codes.OK},
		{"no principal and no status", nil, map[string]string{"q": "x"}, codes.OK},
		{"published explicit, anonymous", principal("anonymous", anon), map[string]string{"status": "published"}, codes.OK},
		{"owner sees own drafts", principal("A", user), map[string]string{"seller_id": "A", "status": "draft"}, codes.OK},
		{"owner view is explicit: seller_id alone is fine (index defaults published)", principal("A", user), map[string]string{"seller_id": "A"}, codes.OK},
		{"another user cannot view someone else's drafts", principal("B", user), map[string]string{"seller_id": "A", "status": "draft"}, codes.PermissionDenied},
		{"draft without seller_id", principal("A", user), map[string]string{"status": "draft"}, codes.PermissionDenied},
		{"draft with empty seller_id", principal("A", user), map[string]string{"status": "draft", "seller_id": ""}, codes.PermissionDenied},
		{"draft by service principal", principal("svc", svc), map[string]string{"seller_id": "svc", "status": "draft"}, codes.PermissionDenied},
		{"draft by anonymous", principal("anonymous", anon), map[string]string{"status": "draft"}, codes.Unauthenticated},
		{"draft by anonymous with matching seller_id", principal("anonymous", anon), map[string]string{"status": "draft", "seller_id": "anonymous"}, codes.Unauthenticated},
		{"draft by unspecified type", principal("A", unspec), map[string]string{"seller_id": "A", "status": "draft"}, codes.Unauthenticated},
		{"draft by user with reserved anonymous id", principal("anonymous", user), map[string]string{"seller_id": "anonymous", "status": "draft"}, codes.Unauthenticated},
		{"draft with no principal", nil, map[string]string{"status": "draft"}, codes.Unauthenticated},
		{"status deleted", principal("A", user), map[string]string{"status": "deleted"}, codes.InvalidArgument},
		{"status rejected", principal("A", user), map[string]string{"status": "rejected"}, codes.InvalidArgument},
		{"status any", principal("A", user), map[string]string{"status": "any"}, codes.InvalidArgument},
		{"status empty", principal("A", user), map[string]string{"status": ""}, codes.InvalidArgument},
		{"in_stock true", principal("anonymous", anon), map[string]string{"in_stock": "true"}, codes.OK},
		{"in_stock maybe", principal("A", user), map[string]string{"in_stock": "maybe"}, codes.InvalidArgument},
		{"in_stock false", principal("A", user), map[string]string{"in_stock": "false"}, codes.InvalidArgument},
		{"in_stock empty", principal("A", user), map[string]string{"in_stock": ""}, codes.InvalidArgument},
		{"status deleted beats auth checks (anonymous)", principal("anonymous", anon), map[string]string{"status": "deleted"}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := map[string]string{}
			for k, v := range tc.filters {
				in[k] = v
			}
			out, err := effectiveFilters(tc.p, in)
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.want)
			}
			if !reflect.DeepEqual(in, mapOrEmpty(tc.filters)) {
				t.Fatalf("input map was mutated: %v -> %v", tc.filters, in)
			}
			if tc.want == codes.OK {
				if !reflect.DeepEqual(out, mapOrEmpty(tc.filters)) {
					t.Fatalf("accepted filters changed: %v -> %v", tc.filters, out)
				}
				out["__mut"] = "x"
				if _, leaked := in["__mut"]; leaked {
					t.Fatal("result aliases the request map")
				}
			} else if out != nil {
				t.Fatalf("a rejected call must return no filters, got %v", out)
			}
		})
	}
}

func mapOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
