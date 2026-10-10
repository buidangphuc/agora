package handler_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

func seedReviews(t *testing.T, n int) *handler.EngagementHandler {
	t.Helper()
	repo := repository.NewInMemoryReviewRepository()
	svc := service.NewReviewService(repo, nil, nil)
	for i := 1; i <= n; i++ {
		rating := int32(i%5) + 1
		if _, err := svc.CreateReview(context.Background(), "l1", fmt.Sprintf("u%d", i), "U", "", rating, fmt.Sprintf("c%03d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	return handler.NewEngagementHandler(repository.NewInMemoryRepository(), svc, nil, nil, nil)
}

func listPage(t *testing.T, h *handler.EngagementHandler, cursor string, size int32, rating int32) *engagementv1.ListReviewsResponse {
	t.Helper()
	res, err := h.ListReviews(context.Background(), &engagementv1.ListReviewsRequest{
		ListingId: "l1", RatingFilter: rating,
		Page: &commonv1.PageRequest{Cursor: cursor, PageSize: size},
	})
	if err != nil {
		t.Fatalf("ListReviews(%q,%d): %v", cursor, size, err)
	}
	return res
}

func TestListReviewsWalksEveryReviewOnceNewestFirst(t *testing.T) {
	h := seedReviews(t, 105)
	var got []string
	cursor, pages := "", 0
	for {
		res := listPage(t, h, cursor, 10, 0)
		pages++
		if res.GetPage().GetTotal() != 105 {
			t.Fatalf("total = %d", res.GetPage().GetTotal())
		}
		for _, r := range res.GetReviews() {
			got = append(got, r.GetComment())
		}
		cursor = res.GetPage().GetNextCursor()
		if cursor == "" {
			if n := len(res.GetReviews()); n != 5 {
				t.Fatalf("last page has %d reviews", n)
			}
			break
		}
	}
	if pages != 11 || len(got) != 105 {
		t.Fatalf("pages=%d reviews=%d", pages, len(got))
	}
	for i, c := range got { // newest (c105) first, oldest (c001) last
		if want := fmt.Sprintf("c%03d", 105-i); c != want {
			t.Fatalf("position %d = %s, want %s", i, c, want)
		}
	}
}

func TestListReviewsPageSizeBoundsAndOffsetPastEnd(t *testing.T) {
	h := seedReviews(t, 105)
	if n := len(listPage(t, h, "", 500, 0).GetReviews()); n != 100 {
		t.Fatalf("page_size 500 returned %d, want 100", n)
	}
	if n := len(listPage(t, h, "", 0, 0).GetReviews()); n != 20 {
		t.Fatalf("default page returned %d, want 20", n)
	}
	res := listPage(t, h, "1000", 10, 0)
	if len(res.GetReviews()) != 0 || res.GetPage().GetNextCursor() != "" || res.GetPage().GetTotal() != 105 {
		t.Fatalf("past-the-end page = %+v", res)
	}
}

func TestListReviewsTotalFollowsRatingFilter(t *testing.T) {
	h := seedReviews(t, 105) // 21 reviews per star
	res := listPage(t, h, "20", 10, 3)
	if res.GetPage().GetTotal() != 21 || len(res.GetReviews()) != 1 || res.GetPage().GetNextCursor() != "" {
		t.Fatalf("filtered page = total %d, %d reviews, next %q",
			res.GetPage().GetTotal(), len(res.GetReviews()), res.GetPage().GetNextCursor())
	}
}

func TestListReviewsRejectsMalformedCursor(t *testing.T) {
	h := seedReviews(t, 3)
	for _, c := range []string{"abc", "-5"} {
		_, err := h.ListReviews(context.Background(), &engagementv1.ListReviewsRequest{
			ListingId: "l1", Page: &commonv1.PageRequest{Cursor: c},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("cursor %q: code = %v", c, status.Code(err))
		}
	}
}
