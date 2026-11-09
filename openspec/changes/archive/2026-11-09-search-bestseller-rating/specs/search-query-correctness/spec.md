## REMOVED Requirements

### Requirement: Rating is not offered while no rating is indexed
**Reason**: Search now indexes a rating, so `min_rating` is accepted and the ratings facet is filled.
**Migration**: The rules live in capability `search-bestseller-rating`, requirement "Search accepts a minimum rating and fills the ratings facet". The e2e scenarios "A minimum-rating search is rejected" and "The ratings facet is empty" in `search/search_query_correctness.feature` are rewritten, not deleted.
