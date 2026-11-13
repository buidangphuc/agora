package listing

// STUB(T2 red): replaced by the implementer.
func (s *ListingService) WithEntitlements(p EntitlementsPort) *ListingService {
	s.ent = p
	return s
}
