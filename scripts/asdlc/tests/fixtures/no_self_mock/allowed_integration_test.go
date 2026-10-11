package x_test

// asdlc: allow-mock external payment gateway
type stubPay struct{}

// asdlc: allow-mock external LLM API
func (s *stubPay) Complete(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) { return nil, nil }
