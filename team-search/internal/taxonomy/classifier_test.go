package taxonomy_test

import (
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"
	aiv1 "github.com/buidangphuc/team-search/generated/platform/ai/v1"
	"github.com/buidangphuc/team-search/internal/taxonomy"
)

type fakeAI struct {
	aiv1.UnimplementedAIServiceServer
	gotReq *aiv1.ClassifyTagsRequest
	gotMD  metadata.MD
	resp   *aiv1.ClassifyTagsResponse
	err    error
	delay  time.Duration
}

func (f *fakeAI) ClassifyTags(ctx context.Context, r *aiv1.ClassifyTagsRequest) (*aiv1.ClassifyTagsResponse, error) {
	f.gotReq = r
	f.gotMD, _ = metadata.FromIncomingContext(ctx)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resp, f.err
}

func serve(t *testing.T, f *fakeAI, timeout time.Duration) *taxonomy.GRPCClassifier {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	aiv1.RegisterAIServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return classifierFor(t, lis.Addr().String(), timeout)
}

func classifierFor(t *testing.T, addr string, timeout time.Duration) *taxonomy.GRPCClassifier {
	t.Helper()
	conn, err := upstream.NewClient(addr, upstream.AsService("service-team-search", "ai.classify"), upstream.WithTimeout(timeout))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return taxonomy.NewGRPCClassifier(conn)
}

func tag(group, slug string) *aiv1.ClassifiedTag {
	return &aiv1.ClassifiedTag{Slug: slug, FacetGroup: group, Confidence: 1}
}

func TestClassify_SendsServicePrincipalAndMapsVariants(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ClassifyTagsResponse{
		Tags: []*aiv1.ClassifiedTag{
			tag("feature", "chong-nuoc-ipx7"), tag("connectivity", "bluetooth-5-3"),
			tag("feature", "Bad Slug"), tag("connectivity", "bluetooth-5-3"),
		},
		Skus: []*aiv1.SkuClassification{
			{VariantId: "v1", Tags: []*aiv1.ClassifiedTag{tag("color", "titan-tu-nhien"), tag("capacity", "256gb")}},
			{VariantId: "v2", Tags: []*aiv1.ClassifiedTag{tag("color", "xanh-navy"), tag("capacity", "512gb"), tag("bad", "Not A Slug")}},
		},
	}}
	c := serve(t, f, time.Second)
	res, err := c.Classify(context.Background(), taxonomy.Listing{
		Title: "iPhone 15 Pro Max", CategoryID: "cat-phones",
		Variants: []taxonomy.Variant{
			{ID: "v1", Name: "Titan Tự Nhiên / 256GB", Price: 30000000, Stock: 5},
			{ID: "v2", Name: "Xanh Navy / 512GB", Price: 34000000, Stock: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.gotMD.Get("x-principal-type"); len(got) != 1 || got[0] != "service" {
		t.Errorf("principal type = %v", got)
	}
	if got := f.gotMD.Get("x-principal-scopes"); len(got) != 1 || got[0] != "ai.classify" {
		t.Errorf("scopes = %v, want the least-privilege ai.classify only", got)
	}
	if got := f.gotMD.Get("x-principal-id"); len(got) != 1 || got[0] != "service-team-search" {
		t.Errorf("principal id = %v", got)
	}
	if f.gotReq.GetTitle() != "iPhone 15 Pro Max" || len(f.gotReq.GetVariants()) != 2 || f.gotReq.GetVariants()[1].GetName() != "Xanh Navy / 512GB" {
		t.Errorf("request = %v", f.gotReq)
	}
	if want := []string{"connectivity:bluetooth-5-3", "feature:chong-nuoc-ipx7"}; !reflect.DeepEqual(res.FacetTags, want) {
		t.Errorf("FacetTags = %v, want %v (sorted, deduped, malformed dropped)", res.FacetTags, want)
	}
	if len(res.SKUs) != 2 {
		t.Fatalf("SKUs = %+v", res.SKUs)
	}
	if want := []string{"capacity:256gb", "color:titan-tu-nhien"}; !reflect.DeepEqual(res.SKUs[0].Attrs, want) || !res.SKUs[0].InStock {
		t.Errorf("sku0 = %+v", res.SKUs[0])
	}
	if want := []string{"capacity:512gb", "color:xanh-navy"}; !reflect.DeepEqual(res.SKUs[1].Attrs, want) || res.SKUs[1].InStock {
		t.Errorf("sku1 = %+v (sold out must not be in stock)", res.SKUs[1])
	}
}

func TestClassify_WithoutVariantsHasNoSkus(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ClassifyTagsResponse{Tags: []*aiv1.ClassifiedTag{tag("connectivity", "bluetooth-5-3")}}}
	res, err := serve(t, f, time.Second).Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe Bluetooth 5.3"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.FacetTags, []string{"connectivity:bluetooth-5-3"}) || len(res.SKUs) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestClassify_ErrorsAreErrorsNotEmptyAnswers(t *testing.T) {
	denied := &fakeAI{err: status.Error(7, "insufficient_scope")}
	if _, err := serve(t, denied, time.Second).Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("a PermissionDenied must be an error so the indexer marks tags_pending")
	}
	slow := &fakeAI{resp: &aiv1.ClassifyTagsResponse{}, delay: 2 * time.Second}
	start := time.Now()
	if _, err := serve(t, slow, 100*time.Millisecond).Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("a call past the deadline must be an error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("deadline not enforced: %v", time.Since(start))
	}
	c := classifierFor(t, "127.0.0.1:1", time.Second)
	if _, err := c.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("unreachable team-ai must be an error")
	}
}

func TestClassify_TooShortTitleIsNothingToClassify(t *testing.T) {
	f := &fakeAI{err: status.Error(13, "must not be called")}
	res, err := serve(t, f, time.Second).Classify(context.Background(), taxonomy.Listing{Title: " a "})
	if err != nil || len(res.FacetTags) != 0 || f.gotReq != nil {
		t.Errorf("res=%+v err=%v called=%v: must not call team-ai", res, err, f.gotReq != nil)
	}
}

// Scenario "The classifier call carries the service principal".
func TestClassifierCallCarriesServicePrincipal(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ClassifyTagsResponse{}}
	c := serve(t, f, time.Second)
	if _, err := c.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe Bluetooth"}); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"x-principal-id":     "service-team-search",
		"x-principal-type":   "service",
		"x-principal-scopes": "ai.classify",
	} {
		if got := f.gotMD.Get(k); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%s]", k, got, want)
		}
	}
}

// Scenario "The classifier call never carries an end-user principal": even when the
// indexing context holds a user principal, team-ai only sees the service one.
func TestClassifierCallNeverCarriesEndUserPrincipal(t *testing.T) {
	f := &fakeAI{resp: &aiv1.ClassifyTagsResponse{}}
	c := serve(t, f, time.Second)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "user-42",
		"x-principal-type", "user",
		"x-principal-scopes", "search:read listing:write",
	))
	if _, err := c.Classify(ctx, taxonomy.Listing{Title: "Tai nghe Bluetooth"}); err != nil {
		t.Fatal(err)
	}
	if got := f.gotMD.Get("x-principal-id"); len(got) != 1 || got[0] != "service-team-search" {
		t.Errorf("x-principal-id = %v, want only the service principal", got)
	}
	if got := f.gotMD.Get("x-principal-type"); len(got) != 1 || got[0] != "service" {
		t.Errorf("x-principal-type = %v, want only service", got)
	}
	if got := f.gotMD.Get("x-principal-scopes"); len(got) != 1 || got[0] != "ai.classify" {
		t.Errorf("x-principal-scopes = %v, want only ai.classify", got)
	}
	for _, v := range f.gotMD.Get("x-principal-id") {
		if strings.Contains(v, "user-42") {
			t.Errorf("end-user id leaked: %v", f.gotMD)
		}
	}
}
