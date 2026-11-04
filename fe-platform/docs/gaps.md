# Backend gaps seen by fe-platform

A screen never invents data the backend does not return. Each row is something a screen needs that the contract or a service does not give yet; the template shows an honest state instead. Add a row here before working around anything.

| ID | Screen | Need | What is missing | Template behaviour | Owner |
|---|---|---|---|---|---|
| G1 | `/products`, `/products/[id]` | Product image | `Listing` carries `image_keys` (storage keys) and no RPC returns a public URL for them (`GetImageUploadUrl` is upload only) | Neutral placeholder tile; cart and order lines use `image_url` when the service fills it | team-listing |
| G2 | `/products?q=` | Search results with title and price | `SearchHit` returns only `listing_id`, `score` and `stock`; there is no batch get for listings | One `GetListing` per hit (first 20 hits only); a hit that is not found is skipped | team-search, team-listing |
| G3 | `/shop/cart`, `/shop/checkout` | Currency for cart lines, subtotal and quotes | `Cart`, `CartItem`, `CheckoutGroupQuote` and `PreviewCheckoutResponse` carry amounts without a currency | Formatted with `DEFAULT_CURRENCY` from `lib/money.ts` (`VND`, the contract's own example) until the contract adds a currency; orders use their own `currency` | team-order |
| G4 | `/shop/confirmation` | One id for a checkout | `CreateOrder` returns one order per seller and there is no checkout or group id | Redirect carries the order ids (opaque, no personal data) and the page shows each order | team-order |
| G5 | forms | Which field failed | Backend errors are a message string; no field list | One form-level message chosen by error kind; the backend message is never put in a URL | all services |
| G6 | `/shop/checkout` | Payment methods offered | `PaymentMethod` lists mock methods next to COD with no way to ask which are enabled | All non-unspecified enum values are offered | team-payment |
| G7 | `/products` | Newest or sorted listings first | `ListListings` has no sort option and the first page of a populated catalogue is not newest-first | The list pages with the response cursor ("Next page"); a new listing can sit several pages in | team-listing |
