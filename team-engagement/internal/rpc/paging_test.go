package rpc_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

// Scenario: Engagement SDK: Question page size is capped at 100
func TestListQuestionsPageSizeClamped(t *testing.T) {
	qa := engagement.NewQAService(storetest.NewFakeQARepository(), nil)
	h := newHandler(t, deps{qa: qa})
	for i := 0; i < 150; i++ {
		if _, err := qa.AskQuestion(context.Background(), "l-1", fmt.Sprintf("u-%d", i), fmt.Sprintf("question %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for size, want := range map[int32]int{0: 20, 100: 100, 101: 100} {
		res, err := h.ListQuestionsByListing(context.Background(), &engagementv1.ListQuestionsByListingRequest{
			ListingId: "l-1", Page: &commonv1.PageRequest{PageSize: size},
		})
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if got := len(res.GetQuestions()); got != want {
			t.Fatalf("size %d: %d items, want %d", size, got, want)
		}
		if got := res.GetPage().GetTotal(); got != 150 {
			t.Fatalf("size %d: total %d, want 150", size, got)
		}
	}
}

// Scenario: Engagement SDK: Malformed report cursor is rejected
func TestListReportsMalformedCursor(t *testing.T) {
	h := newHandler(t, deps{})
	for _, c := range []string{"abc", "-1"} {
		_, err := h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Page: &commonv1.PageRequest{Cursor: c}})
		st := status.Convert(err)
		if st.Code() != codes.InvalidArgument || st.Message() != "invalid cursor" {
			t.Fatalf("cursor %q: got %v %q, want InvalidArgument \"invalid cursor\"", c, st.Code(), st.Message())
		}
	}
}

// Scenario: Engagement SDK: Malformed dispute cursor stays Internal
func TestListDisputesMalformedCursorStaysInternal(t *testing.T) {
	h := newHandler(t, deps{})
	_, err := h.ListDisputes(adminCtx(), &engagementv1.ListDisputesRequest{Page: &commonv1.PageRequest{Cursor: "%%%"}})
	st := status.Convert(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q, want Internal \"internal error\"", st.Code(), st.Message())
	}
}
