package repository

// TEST-STUB (task 7): compile-only. The implementer deletes this file and adds the
// real option to return.go.

// WithReturnRequestedOutbox makes every created return write the row built by b in
// the same transaction as the insert.
func WithReturnRequestedOutbox(b ReturnOutboxBuilder) ReturnRepoOption {
	return func(*returnRepoConfig) {}
}
