package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/rpc"
)

func newHandlerWith(t testing.TB, repo *fakeRepo, logger *slog.Logger) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(chat.NewService(repo, logger), logger)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func setup(t testing.TB) (*rpc.Handler, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	return newHandlerWith(t, repo, slog.New(slog.DiscardHandler)), repo
}

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: auth.TypeUser, Scopes: []string{"chat:read", "chat:write"}})
}

func dial(t *testing.T, h *rpc.Handler) chatv1.ChatServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor()))
	rpc.Register(srv, h)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return chatv1.NewChatServiceClient(conn)
}

func mdCtx(id string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-principal-id", id, "x-principal-type", "user", "x-principal-scopes", "chat:read,chat:write"))
}

func newThread(t *testing.T, h *rpc.Handler, buyer, seller, listing string) string {
	t.Helper()
	res, err := h.GetOrCreateThread(userCtx(buyer), &chatv1.GetOrCreateThreadRequest{SellerId: seller, ListingId: listing})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return res.GetThread().GetId()
}

func wantCode(t *testing.T, label string, err error, want codes.Code) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("%s: code = %v (%v), want %v", label, status.Code(err), err, want)
	}
}

// Scenario: A participant sends and reads a message
func TestChatLifecycleOverGRPC(t *testing.T) {
	h, _ := setup(t)
	client := dial(t, h)

	res, err := client.GetOrCreateThread(mdCtx("buyer-1"), &chatv1.GetOrCreateThreadRequest{SellerId: "seller-1", ListingId: "l-1"})
	if err != nil {
		t.Fatalf("GetOrCreateThread: %v", err)
	}
	threadID := res.GetThread().GetId()
	if _, err := client.SendMessage(mdCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: "hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	got, err := client.GetThreadMessages(mdCtx("seller-1"), &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	if err != nil {
		t.Fatalf("GetThreadMessages: %v", err)
	}
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetContent() != "hello" || got.GetMessages()[0].GetSenderId() != "buyer-1" {
		t.Fatalf("messages = %+v", got.GetMessages())
	}
}

// Scenario: An anonymous caller is rejected
func TestAnonymousAndMissingPrincipalAreUnauthenticated(t *testing.T) {
	h, repo := setup(t)
	client := dial(t, h)
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"anonymous id and type", metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous"))},
		{"anonymous type with an id", metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "anonymous"))},
		{"anonymous id without a type", metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-principal-id", "anonymous"))},
		{"no principal", context.Background()},
	}
	for _, c := range cases {
		_, err := client.ListThreads(c.ctx, &chatv1.ListThreadsRequest{})
		wantCode(t, c.name+" ListThreads", err, codes.Unauthenticated)
		_, err = client.GetOrCreateThread(c.ctx, &chatv1.GetOrCreateThreadRequest{SellerId: "s"})
		wantCode(t, c.name+" GetOrCreateThread", err, codes.Unauthenticated)
		_, err = client.SendMessage(c.ctx, &chatv1.SendMessageRequest{ThreadId: "t", Content: "x"})
		wantCode(t, c.name+" SendMessage", err, codes.Unauthenticated)
		_, err = client.GetThreadMessages(c.ctx, &chatv1.GetThreadMessagesRequest{ThreadId: "t"})
		wantCode(t, c.name+" GetThreadMessages", err, codes.Unauthenticated)
		_, err = client.SearchMessages(c.ctx, &chatv1.SearchMessagesRequest{Query: "x"})
		wantCode(t, c.name+" SearchMessages", err, codes.Unauthenticated)
		_, err = client.MarkThreadRead(c.ctx, &chatv1.MarkThreadReadRequest{ThreadId: "t"})
		wantCode(t, c.name+" MarkThreadRead", err, codes.Unauthenticated)
		_, err = client.ListQuickReplies(c.ctx, &chatv1.ListQuickRepliesRequest{SellerId: "s"})
		wantCode(t, c.name+" ListQuickReplies", err, codes.Unauthenticated)
	}
	if len(repo.threads) != 0 || len(repo.outboxRows()) != 0 {
		t.Fatalf("anonymous calls wrote state: threads=%d rows=%d", len(repo.threads), len(repo.outboxRows()))
	}
}

// Scenario: A non-participant cannot read, send or mark read; A non-participant cannot change another pair's unread count; A non-participant cannot obtain message content
func TestNonParticipantIsDeniedOnEveryThreadRPC(t *testing.T) {
	h, repo := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	if _, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: "private-content"}); err != nil {
		t.Fatal(err)
	}
	rowsBefore := len(repo.outboxRows())
	unreadBefore := repo.thread(threadID).UnreadCountSeller
	intruder := userCtx("intruder-99")

	got, err := h.GetThreadMessages(intruder, &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	wantCode(t, "GetThreadMessages", err, codes.PermissionDenied)
	if got != nil && len(got.GetMessages()) != 0 {
		t.Fatalf("content leaked to non-participant: %+v", got)
	}
	_, err = h.SendMessage(intruder, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "spam"})
	wantCode(t, "SendMessage", err, codes.PermissionDenied)
	_, err = h.MarkThreadRead(intruder, &chatv1.MarkThreadReadRequest{ThreadId: threadID})
	wantCode(t, "MarkThreadRead", err, codes.PermissionDenied)

	if n := len(repo.storedMessages(threadID)); n != 1 {
		t.Fatalf("stored messages = %d, want 1", n)
	}
	if n := len(repo.outboxRows()); n != rowsBefore {
		t.Fatalf("outbox rows = %d, want %d", n, rowsBefore)
	}
	if u := repo.thread(threadID).UnreadCountSeller; u != unreadBefore {
		t.Fatalf("unread count changed from %d to %d", unreadBefore, u)
	}
}

// Scenario: A non-participant cannot obtain message content
func TestPermissionDeniedLeaksNoParticipantIDs(t *testing.T) {
	h, _ := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	intruder := userCtx("intruder-99")
	_, e1 := h.GetThreadMessages(intruder, &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	_, e2 := h.SendMessage(intruder, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "x"})
	_, e3 := h.MarkThreadRead(intruder, &chatv1.MarkThreadReadRequest{ThreadId: threadID})
	for i, err := range []error{e1, e2, e3} {
		msg := status.Convert(err).Message()
		for _, id := range []string{"buyer-1", "seller-1", "intruder-99", threadID} {
			if strings.Contains(msg, id) {
				t.Fatalf("call %d: message %q leaks %q", i, msg, id)
			}
		}
	}
}

// Scenario: A thread with oneself or without a seller is rejected
func TestGetOrCreateThreadRejectsEmptyAndSelfSeller(t *testing.T) {
	h, repo := setup(t)
	_, err := h.GetOrCreateThread(userCtx("buyer-1"), &chatv1.GetOrCreateThreadRequest{SellerId: ""})
	wantCode(t, "empty seller", err, codes.InvalidArgument)
	_, err = h.GetOrCreateThread(userCtx("buyer-1"), &chatv1.GetOrCreateThreadRequest{SellerId: "buyer-1"})
	wantCode(t, "self seller", err, codes.InvalidArgument)
	if len(repo.threads) != 0 {
		t.Fatalf("threads created: %d", len(repo.threads))
	}
}

// Scenario: An empty message is rejected
func TestSendMessageRejectsEmptyContentAndThread(t *testing.T) {
	h, repo := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	_, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: ""})
	wantCode(t, "empty content", err, codes.InvalidArgument)
	_, err = h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: "", Content: "x"})
	wantCode(t, "empty thread", err, codes.InvalidArgument)
	if len(repo.storedMessages(threadID)) != 0 || len(repo.outboxRows()) != 0 {
		t.Fatal("state written by rejected sends")
	}
}

// Scenario: An unknown thread is NotFound for send and mark read
func TestUnknownThreadIsNotFoundForSendAndMarkRead(t *testing.T) {
	h, repo := setup(t)
	_, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: "missing", Content: "x"})
	wantCode(t, "send", err, codes.NotFound)
	_, err = h.MarkThreadRead(userCtx("buyer-1"), &chatv1.MarkThreadReadRequest{ThreadId: "missing"})
	wantCode(t, "mark read", err, codes.NotFound)
	if len(repo.outboxRows()) != 0 {
		t.Fatal("outbox row written for unknown thread")
	}
}

// Scenario: Errors reach callers without internal detail (store.ErrThreadNotFound maps to NotFound)
func TestUnknownThreadIsNotFound(t *testing.T) {
	h, _ := setup(t)
	_, err := h.GetThreadMessages(userCtx("buyer-1"), &chatv1.GetThreadMessagesRequest{ThreadId: "missing"})
	wantCode(t, "GetThreadMessages", err, codes.NotFound)
}

// Scenario: A database error is masked
func TestInternalErrorIsMasked(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	repo := newFakeRepo()
	h := newHandlerWith(t, repo, logger)
	repo.failList = errors.New(`pq: relation "chat_messages" does not exist`)

	_, err := h.ListThreads(userCtx("buyer-1"), &chatv1.ListThreadsRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if strings.Contains(st.Message(), "chat_messages") {
		t.Fatal("cause leaked to caller")
	}
	if !strings.Contains(buf.String(), "chat_messages") {
		t.Fatalf("cause not logged: %s", buf.String())
	}
}

// Scenario: Cancellation and deadline are not Internal
func TestContextErrorsKeepTheirCodes(t *testing.T) {
	h, repo := setup(t)
	repo.failList = context.Canceled
	_, err := h.ListThreads(userCtx("buyer-1"), &chatv1.ListThreadsRequest{})
	wantCode(t, "canceled", err, codes.Canceled)
	repo.failList = context.DeadlineExceeded
	_, err = h.ListThreads(userCtx("buyer-1"), &chatv1.ListThreadsRequest{})
	wantCode(t, "deadline", err, codes.DeadlineExceeded)
}

// Scenario: A unique violation is AlreadyExists
func TestUniqueViolationIsAlreadyExists(t *testing.T) {
	h, repo := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	repo.failSave = fmt.Errorf("save: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"})
	_, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: "x"})
	wantCode(t, "unique violation", err, codes.AlreadyExists)
}

func seedMessageThread(repo *fakeRepo, n int) {
	repo.seedThread("t-1", "buyer-1", "seller-1")
	repo.seedMessages("t-1", "buyer-1", "m", n)
}

func getMsgs(t *testing.T, h *rpc.Handler, cursor string, size int32) *chatv1.GetThreadMessagesResponse {
	t.Helper()
	res, err := h.GetThreadMessages(userCtx("buyer-1"), &chatv1.GetThreadMessagesRequest{
		ThreadId: "t-1",
		Page:     &commonv1.PageRequest{Cursor: cursor, PageSize: size},
	})
	if err != nil {
		t.Fatalf("GetThreadMessages(%q,%d): %v", cursor, size, err)
	}
	return res
}

// Scenario: Page size defaults apply
func TestPageSizeDefaults(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	for i := 0; i < 120; i++ {
		repo.seedThread(fmt.Sprintf("th-%03d", i), "u-1", fmt.Sprintf("s-%03d", i))
	}
	repo.seedMessages("th-000", "u-1", "needle", 120)

	lt, err := h.ListThreads(userCtx("u-1"), &chatv1.ListThreadsRequest{Page: &commonv1.PageRequest{PageSize: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lt.GetThreads()) != 20 || lt.GetPage().GetTotal() != 120 {
		t.Fatalf("ListThreads rows=%d total=%d", len(lt.GetThreads()), lt.GetPage().GetTotal())
	}
	gm := getMsgs(t, h, "", 0)
	if len(gm.GetMessages()) != 50 || gm.GetPage().GetTotal() != 120 {
		t.Fatalf("GetThreadMessages rows=%d total=%d", len(gm.GetMessages()), gm.GetPage().GetTotal())
	}
	sm, err := h.SearchMessages(userCtx("u-1"), &chatv1.SearchMessagesRequest{Query: "needle", Page: &commonv1.PageRequest{PageSize: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sm.GetMessages()) != 50 || sm.GetPage().GetTotal() != 120 {
		t.Fatalf("SearchMessages rows=%d total=%d", len(sm.GetMessages()), sm.GetPage().GetTotal())
	}
}

// Scenario: A page size that exceeds the maximum is rejected in favour of the default
func TestPageSizeAboveMaximumUsesDefault(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	res := getMsgs(t, h, "0", 101)
	if len(res.GetMessages()) != 50 || res.GetMessages()[0].GetContent() != "m-000" {
		t.Fatalf("rows=%d first=%v", len(res.GetMessages()), res.GetMessages())
	}
	if c := repo.lastMsgCall(); c.offset != 0 || c.limit != 50 {
		t.Fatalf("repo call = %+v, want offset 0 limit 50", c)
	}
}

// Scenario: Next cursor uses the effective page size
func TestNextCursorUsesEffectivePageSize(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	res := getMsgs(t, h, "2", 101)
	if len(res.GetMessages()) != 50 || res.GetMessages()[0].GetContent() != "m-050" {
		t.Fatalf("rows=%d", len(res.GetMessages()))
	}
	if c := repo.lastMsgCall(); c.offset != 50 || c.limit != 50 {
		t.Fatalf("repo call = %+v", c)
	}
	if got := res.GetPage().GetNextCursor(); got != "3" {
		t.Fatalf("next_cursor = %q, want 3", got)
	}
}

// Scenario: The page size boundaries are 1 and 100
func TestPageSizeBoundaries(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	r100 := getMsgs(t, h, "", 100)
	if len(r100.GetMessages()) != 100 || r100.GetPage().GetNextCursor() != "2" {
		t.Fatalf("size 100: rows=%d next=%q", len(r100.GetMessages()), r100.GetPage().GetNextCursor())
	}
	if r1 := getMsgs(t, h, "", 1); len(r1.GetMessages()) != 1 {
		t.Fatalf("size 1: rows=%d", len(r1.GetMessages()))
	}
	if r0 := getMsgs(t, h, "", 0); len(r0.GetMessages()) != 50 {
		t.Fatalf("size 0: rows=%d", len(r0.GetMessages()))
	}
}

// Scenario: ListThreads paginates with its own default
func TestListThreadsPagination(t *testing.T) {
	h, repo := setup(t)
	for i := 0; i < 50; i++ {
		repo.seedThread(fmt.Sprintf("th-%03d", i), "u-1", fmt.Sprintf("s-%03d", i))
	}
	a, err := h.ListThreads(userCtx("u-1"), &chatv1.ListThreadsRequest{Page: &commonv1.PageRequest{Cursor: "2", PageSize: 101}})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.GetThreads()) != 20 || a.GetThreads()[0].GetId() != "th-020" || a.GetPage().GetNextCursor() != "3" {
		t.Fatalf("first: rows=%d next=%q", len(a.GetThreads()), a.GetPage().GetNextCursor())
	}
	if c := repo.lastListCall(); c.offset != 20 || c.limit != 20 {
		t.Fatalf("repo call = %+v", c)
	}
	b, err := h.ListThreads(userCtx("u-1"), &chatv1.ListThreadsRequest{Page: &commonv1.PageRequest{Cursor: "3", PageSize: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.GetThreads()) != 10 || b.GetPage().GetNextCursor() != "" {
		t.Fatalf("second: rows=%d next=%q", len(b.GetThreads()), b.GetPage().GetNextCursor())
	}
}

// Scenario: SearchMessages paginates with its own default
func TestSearchMessagesPagination(t *testing.T) {
	h, repo := setup(t)
	repo.seedThread("t-1", "u-1", "s-1")
	repo.seedMessages("t-1", "u-1", "needle", 120)
	a, err := h.SearchMessages(userCtx("u-1"), &chatv1.SearchMessagesRequest{Query: "needle", Page: &commonv1.PageRequest{Cursor: "2", PageSize: 101}})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.GetMessages()) != 50 || a.GetMessages()[0].GetContent() != "needle-050" || a.GetPage().GetNextCursor() != "3" {
		t.Fatalf("first: rows=%d next=%q", len(a.GetMessages()), a.GetPage().GetNextCursor())
	}
	if c := repo.lastSearchCall(); c.offset != 50 || c.limit != 50 {
		t.Fatalf("repo call = %+v", c)
	}
	b, err := h.SearchMessages(userCtx("u-1"), &chatv1.SearchMessagesRequest{Query: "needle", Page: &commonv1.PageRequest{Cursor: "abc", PageSize: -1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.GetMessages()) != 50 || b.GetMessages()[0].GetContent() != "needle-000" || b.GetPage().GetNextCursor() != "2" {
		t.Fatalf("second: rows=%d next=%q", len(b.GetMessages()), b.GetPage().GetNextCursor())
	}
}

// Scenario: The cursor is the next page number
func TestNextCursorIsNextPageNumber(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	p1 := getMsgs(t, h, "", 50)
	if p1.GetPage().GetNextCursor() != "2" {
		t.Fatalf("page 1 next = %q", p1.GetPage().GetNextCursor())
	}
	p2 := getMsgs(t, h, p1.GetPage().GetNextCursor(), 50)
	if p2.GetPage().GetNextCursor() != "3" {
		t.Fatalf("page 2 next = %q", p2.GetPage().GetNextCursor())
	}
	p3 := getMsgs(t, h, p2.GetPage().GetNextCursor(), 50)
	if len(p3.GetMessages()) != 20 || p3.GetPage().GetNextCursor() != "" {
		t.Fatalf("page 3 rows=%d next=%q", len(p3.GetMessages()), p3.GetPage().GetNextCursor())
	}
}

// Scenario: Bad cursor and negative page size fall back
func TestBadCursorAndNegativePageSizeFallBack(t *testing.T) {
	h, repo := setup(t)
	seedMessageThread(repo, 120)
	for _, cursor := range []string{"abc", "0", "-3"} {
		res := getMsgs(t, h, cursor, -1)
		if len(res.GetMessages()) != 50 || res.GetMessages()[0].GetContent() != "m-000" || res.GetPage().GetNextCursor() != "2" {
			t.Fatalf("cursor %q: rows=%d next=%q", cursor, len(res.GetMessages()), res.GetPage().GetNextCursor())
		}
		if c := repo.lastMsgCall(); c.offset != 0 || c.limit != 50 {
			t.Fatalf("cursor %q: repo call = %+v", cursor, c)
		}
	}
}

// Scenario: Quick replies default to the caller and to seeded defaults
func TestChatHandler_ListQuickReplies(t *testing.T) {
	h, repo := setup(t)
	_, err := h.ListQuickReplies(context.Background(), &chatv1.ListQuickRepliesRequest{SellerId: "seller-1"})
	wantCode(t, "anonymous", err, codes.Unauthenticated)

	res, err := h.ListQuickReplies(userCtx("seller-1"), &chatv1.ListQuickRepliesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetQuickReplies()) != 2 || res.GetQuickReplies()[0] != "default-1" {
		t.Fatalf("defaults = %v", res.GetQuickReplies())
	}
	if len(repo.quickAsked) != 1 || repo.quickAsked[0] != "seller-1" {
		t.Fatalf("asked for %v, want the caller's id", repo.quickAsked)
	}

	repo.setQuickReplies("seller-2", []string{"Còn hàng bạn nhé"})
	res, err = h.ListQuickReplies(userCtx("buyer-1"), &chatv1.ListQuickRepliesRequest{SellerId: "seller-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetQuickReplies()) != 1 || res.GetQuickReplies()[0] != "Còn hàng bạn nhé" {
		t.Fatalf("configured = %v", res.GetQuickReplies())
	}
}

// Scenario: Listing card and quick reply messages round-trip
func TestChatHandler_SendListingCardRoundtrip(t *testing.T) {
	h, repo := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	ctx := userCtx("buyer-1")
	payload := `{"title":"Căn hộ 2PN Quận 7","price":3200000000}`
	res, err := h.SendMessage(ctx, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "xem tin", MessageType: chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD, ListingId: "listing-42", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetMessage().GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD || res.GetMessage().GetListingId() != "listing-42" || res.GetMessage().GetPayload() != payload {
		t.Fatalf("send response = %+v", res.GetMessage())
	}
	if _, err := h.SendMessage(ctx, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "ok", MessageType: chatv1.MessageType_MESSAGE_TYPE_QUICK_REPLY, Payload: `{"k":1}`}); err != nil {
		t.Fatal(err)
	}
	got, err := h.GetThreadMessages(ctx, &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetMessages()) != 2 {
		t.Fatalf("messages = %d", len(got.GetMessages()))
	}
	a, b := got.GetMessages()[0], got.GetMessages()[1]
	if a.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD || a.GetListingId() != "listing-42" || a.GetPayload() != payload {
		t.Fatalf("card did not round-trip: %+v", a)
	}
	if b.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_QUICK_REPLY || b.GetPayload() != `{"k":1}` {
		t.Fatalf("quick reply did not round-trip: %+v", b)
	}
	if len(repo.outboxRows()) != 2 {
		t.Fatalf("outbox rows = %d", len(repo.outboxRows()))
	}
}

// Scenario: A plain text message keeps working
func TestChatHandler_SendPlainTextBackCompat(t *testing.T) {
	h, _ := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	res, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: "Xin chào shop"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.GetMessage()
	if m.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_TEXT || m.GetListingId() != "" || m.GetPayload() != "" || m.GetContent() != "Xin chào shop" {
		t.Fatalf("message = %+v", m)
	}
}

// Scenario: Marking read clears the unread count
func TestChatHandler_GetThreadMessagesAndMarkRead(t *testing.T) {
	h, _ := setup(t)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	for _, c := range []string{"one", "two"} {
		if _, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: c}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(user string) *chatv1.ChatThread {
		res, err := h.ListThreads(userCtx(user), &chatv1.ListThreadsRequest{})
		if err != nil || len(res.GetThreads()) != 1 {
			t.Fatalf("ListThreads(%s): %v %v", user, res, err)
		}
		return res.GetThreads()[0]
	}
	if th := list("seller-1"); th.GetUnreadCountSeller() != 2 || th.GetUnreadCountBuyer() != 0 {
		t.Fatalf("before: seller=%d buyer=%d", th.GetUnreadCountSeller(), th.GetUnreadCountBuyer())
	}
	if _, err := h.MarkThreadRead(userCtx("seller-1"), &chatv1.MarkThreadReadRequest{ThreadId: threadID}); err != nil {
		t.Fatal(err)
	}
	if th := list("seller-1"); th.GetUnreadCountSeller() != 0 || th.GetUnreadCountBuyer() != 0 {
		t.Fatalf("after: seller=%d buyer=%d", th.GetUnreadCountSeller(), th.GetUnreadCountBuyer())
	}
	msgs, err := h.GetThreadMessages(userCtx("buyer-1"), &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	if err != nil || len(msgs.GetMessages()) != 2 || msgs.GetMessages()[0].GetContent() != "one" || msgs.GetMessages()[1].GetContent() != "two" {
		t.Fatalf("messages = %+v err=%v", msgs, err)
	}
}

// Scenario: Thread list reports last message and unread
func TestChatHandler_ListThreads(t *testing.T) {
	h, _ := setup(t)
	_, err := h.ListThreads(context.Background(), &chatv1.ListThreadsRequest{})
	wantCode(t, "anonymous", err, codes.Unauthenticated)
	threadID := newThread(t, h, "buyer-1", "seller-1", "l-1")
	newThread(t, h, "buyer-1", "seller-2", "l-2")
	for _, c := range []string{"a", "b"} {
		if _, err := h.SendMessage(userCtx("buyer-1"), &chatv1.SendMessageRequest{ThreadId: threadID, Content: c}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := h.ListThreads(userCtx("seller-1"), &chatv1.ListThreadsRequest{})
	if err != nil || len(res.GetThreads()) != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	th := res.GetThreads()[0]
	if th.GetLastMessageText() != "b" || th.GetLastMessageAt() == nil || th.GetUnreadCountSeller() != 2 {
		t.Fatalf("thread = %+v", th)
	}
	buyer, err := h.ListThreads(userCtx("buyer-1"), &chatv1.ListThreadsRequest{})
	if err != nil || len(buyer.GetThreads()) != 2 || buyer.GetPage().GetTotal() != 2 {
		t.Fatalf("buyer res=%v err=%v", buyer, err)
	}
}

// Scenario: The same thread is returned for the same triple
func TestChatHandler_GetOrCreateThread(t *testing.T) {
	h, _ := setup(t)
	_, err := h.GetOrCreateThread(context.Background(), &chatv1.GetOrCreateThreadRequest{SellerId: "seller-1"})
	wantCode(t, "anonymous", err, codes.Unauthenticated)
	a := newThread(t, h, "buyer-1", "seller-1", "l-1")
	b := newThread(t, h, "buyer-1", "seller-1", "l-1")
	c := newThread(t, h, "buyer-1", "seller-1", "l-2")
	if a != b || a == c {
		t.Fatalf("ids a=%s b=%s c=%s", a, b, c)
	}
	res, err := h.GetOrCreateThread(userCtx("buyer-1"), &chatv1.GetOrCreateThreadRequest{SellerId: "seller-1", ListingId: "l-1"})
	if err != nil || res.GetThread().GetBuyerId() != "buyer-1" || res.GetThread().GetSellerId() != "seller-1" {
		t.Fatalf("thread = %+v err=%v", res.GetThread(), err)
	}
}

// Scenario: The sender name is derived from the id; A short id gets the fallback sender name
func TestSenderNameIsDerivedFromID(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"abcdef123456", "User abcdef"},
		{"abcdef", "Người dùng"},
		{"abcdefg", "User abcdef"},
	}
	for _, c := range cases {
		h, repo := setup(t)
		threadID := newThread(t, h, c.id, "seller-1", "l-1")
		if _, err := h.SendMessage(userCtx(c.id), &chatv1.SendMessageRequest{ThreadId: threadID, Content: "hi"}); err != nil {
			t.Fatal(err)
		}
		msgs := repo.storedMessages(threadID)
		if len(msgs) != 1 || msgs[0].SenderName != c.want {
			t.Fatalf("id %q: stored %+v, want sender_name %q", c.id, msgs, c.want)
		}
	}
}

// Scenario: A blank search returns nothing
func TestBlankSearchReturnsEmptyWithoutQuery(t *testing.T) {
	h, repo := setup(t)
	res, err := h.SearchMessages(userCtx("buyer-1"), &chatv1.SearchMessagesRequest{Query: "   "})
	if err != nil || len(res.GetMessages()) != 0 || res.GetPage().GetTotal() != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if len(repo.searchCalls) != 0 {
		t.Fatal("repository searched for a blank query")
	}
}
