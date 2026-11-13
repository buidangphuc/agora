package store

// ListingFilter narrows ListByOwner. Statuses are OR-ed stored status strings
// (empty means any); Query is a literal, case-insensitive title substring
// (empty means any); the two combine with AND.
type ListingFilter struct {
	Statuses []string
	Query    string
}
