package auditx_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type fakeWriter struct {
	err    error
	block  bool
	events []auditx.Event
	order  *[]string
}

func (f *fakeWriter) WriteAuditEvent(ctx context.Context, ev auditx.Event) error {
	if f.order != nil {
		*f.order = append(*f.order, "write")
	}
	f.events = append(f.events, ev)
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func quietLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func requireUnavailable(t *testing.T, err error) {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok || err == nil {
		t.Fatalf("err = %v, want a gRPC status", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", st.Code())
	}
	if st.Message() != "audit trail unavailable" {
		t.Errorf("message = %q, want %q", st.Message(), "audit trail unavailable")
	}
}

func TestDoWritesBeforeAction(t *testing.T) {
	var order []string
	w := &fakeWriter{order: &order}
	g := auditx.NewGate(w, quietLogger(&bytes.Buffer{}))
	ev := auditx.Event{ActorID: "admin-1", Action: "voucher.create", TargetType: "voucher", TargetID: "v-1", Metadata: map[string]string{"k": "v"}}
	runs := 0
	err := g.Do(context.Background(), ev, func(context.Context) error {
		runs++
		order = append(order, "action")
		return nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if runs != 1 {
		t.Errorf("action ran %d times, want 1", runs)
	}
	if len(order) != 2 || order[0] != "write" || order[1] != "action" {
		t.Errorf("order = %v, want [write action]", order)
	}
	if len(w.events) != 1 || w.events[0].ActorID != "admin-1" || w.events[0].Action != "voucher.create" ||
		w.events[0].TargetType != "voucher" || w.events[0].TargetID != "v-1" || w.events[0].Metadata["k"] != "v" {
		t.Errorf("writer saw %+v, want the event passed to Do", w.events)
	}
}

func TestDoFailsClosedOnWriteError(t *testing.T) {
	var logs bytes.Buffer
	w := &fakeWriter{err: errors.New("dial tcp 10.0.0.9: refused")}
	g := auditx.NewGate(w, quietLogger(&logs))
	runs := 0
	err := g.Do(context.Background(), auditx.Event{Action: "voucher.create", TargetID: "v-1"}, func(context.Context) error {
		runs++
		return nil
	})
	if runs != 0 {
		t.Errorf("action ran %d times, want 0", runs)
	}
	requireUnavailable(t, err)
	if strings.Contains(err.Error(), "10.0.0.9") {
		t.Errorf("error %q leaks the cause", err)
	}
	if !strings.Contains(logs.String(), "code=Unknown") {
		t.Errorf("log %q does not carry the status code", logs.String())
	}
}

func TestDoFailureLogsNoTargetOrErrorText(t *testing.T) {
	const targetID = "listing-8f3a9c"
	const errText = "dial tcp 10.0.0.9: refused"
	ev := auditx.Event{ActorID: "admin-1", Action: "voucher.create", TargetType: "voucher", TargetID: targetID}

	var failed bytes.Buffer
	g := auditx.NewGate(&fakeWriter{err: status.Error(codes.Unavailable, errText)}, quietLogger(&failed))
	requireUnavailable(t, g.Do(context.Background(), ev, func(context.Context) error { return nil }))

	var skipped bytes.Buffer
	g = auditx.NewGate(nil, quietLogger(&skipped))
	requireUnavailable(t, g.Do(context.Background(), ev, func(context.Context) error { return nil }))

	for name, logs := range map[string]string{"write failed": failed.String(), "no writer": skipped.String()} {
		for _, leak := range []string{targetID, errText, "10.0.0.9", "admin-1"} {
			if strings.Contains(logs, leak) {
				t.Errorf("%s log %q contains %q", name, logs, leak)
			}
		}
		for _, want := range []string{"action=voucher.create", "target_type=voucher"} {
			if !strings.Contains(logs, want) {
				t.Errorf("%s log %q lacks %q", name, logs, want)
			}
		}
	}
	if !strings.Contains(failed.String(), "code=Unavailable") {
		t.Errorf("write failed log %q lacks the status code", failed.String())
	}
}

func TestDoFailsClosedWithoutWriter(t *testing.T) {
	g := auditx.NewGate(nil, quietLogger(&bytes.Buffer{}))
	runs := 0
	err := g.Do(context.Background(), auditx.Event{Action: "a"}, func(context.Context) error {
		runs++
		return nil
	})
	if runs != 0 {
		t.Errorf("action ran %d times, want 0", runs)
	}
	requireUnavailable(t, err)
}

func TestDoFailsClosedOnTimeout(t *testing.T) {
	g := auditx.NewGate(&fakeWriter{block: true}, quietLogger(&bytes.Buffer{}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runs := 0
	start := time.Now()
	err := g.Do(ctx, auditx.Event{Action: "a"}, func(context.Context) error {
		runs++
		return nil
	})
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("Do took %v, want under 500ms", d)
	}
	if runs != 0 {
		t.Errorf("action ran %d times, want 0", runs)
	}
	requireUnavailable(t, err)
}

func TestDoBoundsWriteWithoutCallerDeadline(t *testing.T) {
	g := auditx.NewGate(&fakeWriter{block: true}, quietLogger(&bytes.Buffer{}))
	runs := 0
	start := time.Now()
	err := g.Do(context.Background(), auditx.Event{Action: "a"}, func(context.Context) error {
		runs++
		return nil
	})
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Errorf("Do took %v, want under 2.5s", d)
	}
	if runs != 0 {
		t.Errorf("action ran %d times, want 0", runs)
	}
	requireUnavailable(t, err)
}

func TestDoReturnsActionErrorUnchanged(t *testing.T) {
	g := auditx.NewGate(&fakeWriter{}, quietLogger(&bytes.Buffer{}))
	want := status.Error(codes.PermissionDenied, "missing scope")
	err := g.Do(context.Background(), auditx.Event{Action: "a"}, func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want the action error itself", err)
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestDialSendsServicePrincipal(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan metadata.MD, 1)
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		got <- md
		var in emptypb.Empty
		if err := stream.RecvMsg(&in); err != nil {
			return err
		}
		return stream.SendMsg(&emptypb.Empty{})
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := auditx.Dial(config.Audit{Addr: lis.Addr().String()}, "team-order", "audit.write")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned a nil connection for a non-empty address")
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Invoke(ctx, "/test.Echo/Call", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	md := <-got
	for key, want := range map[string]string{
		auth.MDPrincipalID:     "team-order",
		auth.MDPrincipalType:   "service",
		auth.MDPrincipalScopes: "audit.write",
	} {
		if v := md.Get(key); len(v) != 1 || v[0] != want {
			t.Errorf("%s = %v, want [%s]", key, v, want)
		}
	}
}

func TestDialServicePrincipalReplacesCallerPrincipal(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan metadata.MD, 1)
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		got <- md
		var in emptypb.Empty
		if err := stream.RecvMsg(&in); err != nil {
			return err
		}
		return stream.SendMsg(&emptypb.Empty{})
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := auditx.Dial(config.Audit{Addr: lis.Addr().String()}, "team-order", "audit.write")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx,
		auth.MDPrincipalID, "buyer",
		auth.MDPrincipalType, "user",
		auth.MDPrincipalScopes, "orders.read")
	if err := conn.Invoke(ctx, "/test.Echo/Call", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	md := <-got
	for key, want := range map[string]string{
		auth.MDPrincipalID:     "team-order",
		auth.MDPrincipalType:   "service",
		auth.MDPrincipalScopes: "audit.write",
	} {
		if v := md.Get(key); len(v) != 1 || v[0] != want {
			t.Errorf("%s = %v, want exactly [%s]", key, v, want)
		}
	}
}

func TestDialEmptyAddressReturnsNilConnection(t *testing.T) {
	conn, err := auditx.Dial(config.Audit{}, "team-order", "audit.write")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if conn != nil {
		t.Errorf("conn = %v, want nil", conn)
	}
}
