package edge

import (
	"context"
	"errors"
	"io"

	"connectrpc.com/connect"

	chatv1 "github.com/buidangphuc/team-gateway/generated/platform/chat/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/chat/v1/chatv1connect"
)

type ChatForwarder struct {
	chatv1connect.UnimplementedChatServiceHandler
	client chatv1.ChatServiceClient
	// aiChat is team-ai's ChatService, used only for StreamChat (AI token stream).
	aiChat chatv1.ChatServiceClient
	edge   *Edge
}

func NewChatForwarder(client, aiChat chatv1.ChatServiceClient, edge *Edge) *ChatForwarder {
	return &ChatForwarder{client: client, aiChat: aiChat, edge: edge}
}

// StreamChat forwards the AI token stream from team-ai's ChatService to the
// browser (via Connect server-streaming). No callRead timeout wrapper — this is
// a long-lived stream; each upstream delta is relayed as it arrives.
func (f *ChatForwarder) StreamChat(
	ctx context.Context,
	req *connect.Request[chatv1.StreamChatRequest],
	stream *connect.ServerStream[chatv1.StreamChatResponse],
) error {
	up, err := f.aiChat.StreamChat(f.edge.outgoing(ctx, req.Header()), req.Msg)
	if err != nil {
		return toConnectErr(err)
	}
	for {
		msg, err := up.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return toConnectErr(err)
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
}

func (f *ChatForwarder) GetOrCreateThread(
	ctx context.Context,
	req *connect.Request[chatv1.GetOrCreateThreadRequest],
) (*connect.Response[chatv1.GetOrCreateThreadResponse], error) {
	var out *chatv1.GetOrCreateThreadResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetOrCreateThread(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ChatForwarder) ListThreads(
	ctx context.Context,
	req *connect.Request[chatv1.ListThreadsRequest],
) (*connect.Response[chatv1.ListThreadsResponse], error) {
	var out *chatv1.ListThreadsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListThreads(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ChatForwarder) GetThreadMessages(
	ctx context.Context,
	req *connect.Request[chatv1.GetThreadMessagesRequest],
) (*connect.Response[chatv1.GetThreadMessagesResponse], error) {
	var out *chatv1.GetThreadMessagesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetThreadMessages(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ChatForwarder) SearchMessages(
	ctx context.Context,
	req *connect.Request[chatv1.SearchMessagesRequest],
) (*connect.Response[chatv1.SearchMessagesResponse], error) {
	var out *chatv1.SearchMessagesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SearchMessages(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ChatForwarder) SendMessage(
	ctx context.Context,
	req *connect.Request[chatv1.SendMessageRequest],
) (*connect.Response[chatv1.SendMessageResponse], error) {
	var out *chatv1.SendMessageResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SendMessage(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ChatForwarder) MarkThreadRead(
	ctx context.Context,
	req *connect.Request[chatv1.MarkThreadReadRequest],
) (*connect.Response[chatv1.MarkThreadReadResponse], error) {
	var out *chatv1.MarkThreadReadResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.MarkThreadRead(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListQuickReplies returns a seller's canned reply templates (read).
func (f *ChatForwarder) ListQuickReplies(
	ctx context.Context,
	req *connect.Request[chatv1.ListQuickRepliesRequest],
) (*connect.Response[chatv1.ListQuickRepliesResponse], error) {
	var out *chatv1.ListQuickRepliesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListQuickReplies(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
