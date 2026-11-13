## ADDED Requirements

### Requirement: A seller brands the shop from /seller/shop

`/seller/shop` SHALL show, besides the shop name, a banner uploader, a tagline field (<= 120 characters, no control characters), a featured-listings picker (at most 6, ordered) and a theme choice with exactly `default`, `dark` and `festive`, pre-filled from `GetStorefront`. One Save SHALL call `UpsertStorefront` once with the full current storefront (slug, display name, banner, tagline, featured ids, theme) so that saving one field never clears another. The seller id SHALL come from the session and never from the form. Saving SHALL show the saved values and the toast "Đã lưu thông tin gian hàng". The page SHALL render nothing but the access guard for a principal without `listing.write`.

#### Scenario: Seller shop UI: the form is pre-filled from the storefront
- **WHEN** `/seller/shop` renders for a seller whose storefront has banner B, tagline "Hoa tươi mỗi ngày", featured ids [a, b] and theme `festive`
- **THEN** the form shows B as the banner preview, "Hoa tươi mỗi ngày" in the tagline field, the titles of listings a and b in that order, and the theme `festive` selected
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopProfileForm.test.tsx` `is pre-filled with the current storefront`

#### Scenario: Seller shop UI: saving sends the full storefront once
- **WHEN** the seller changes only the tagline and clicks "Lưu thay đổi"
- **THEN** `UpsertStorefront` is called exactly once with the unchanged slug, display name, banner, featured ids and theme and the new tagline, and the seller id equals the session principal id
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `upsertStorefrontAction keeps unchanged fields`

#### Scenario: Seller shop UI: a tampered seller id is ignored
- **WHEN** the action is invoked with a `sellerId` field of another seller in its input
- **THEN** `UpsertStorefront` is called with the session principal id and the foreign id appears in no RPC
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `seller id comes from the session`

#### Scenario: Seller shop UI: a principal without listing.write cannot save
- **WHEN** `upsertStorefrontAction` runs with no session, and again with a session lacking `listing.write`
- **THEN** both calls return the error "Bạn cần đăng nhập để cập nhật gian hàng." or the access-denied error respectively and `UpsertStorefront` is called zero times
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `storefront save refused without scope`

#### Scenario: Seller shop UI: a failed read blocks the save
- **WHEN** the current storefront cannot be read because `GetStorefront` answers UNAVAILABLE
- **THEN** the action returns "Không tải được hồ sơ gian hàng hiện tại. Vui lòng thử lại." and `UpsertStorefront` is called zero times
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `read failure blocks the save`

#### Scenario: Seller shop UI: a seller brands the shop in the browser and the buyer sees it
- **WHEN** a seller opens `/seller/shop`, uploads a banner, sets tagline "Hoa tươi mỗi ngày", picks two published listings and the theme `dark`, saves, and then reloads `/seller/shop`
- **THEN** the reloaded form shows the same banner, tagline, the two listings in the chosen order and `dark`, and the buyer app `/shop/{sellerId}` shows the tagline "Hoa tươi mỗi ngày"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_shop_customisation_ui.feature` `Seller shop UI: a seller brands the shop in the browser`

### Requirement: The tagline and theme are validated before sending

The form SHALL block submit and show an inline error when the tagline is longer than 120 characters (counted as Unicode code points after trimming) or contains a control character, and when the theme is not one of `default`, `dark`, `festive`. An empty tagline SHALL be valid. The server answer remains authoritative: an `INVALID_ARGUMENT` from the action SHALL be shown as an alert with the server message and the form SHALL keep the seller's input.

#### Scenario: Seller shop UI: a 120-character tagline is accepted and 121 is refused
- **WHEN** the seller enters a tagline of exactly 120 characters and submits, then enters 121 characters and submits
- **THEN** the first submit calls the action once; the second shows "Tagline tối đa 120 ký tự." and calls the action zero more times
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/shopBranding.test.ts` `tagline boundary at 120`

#### Scenario: Seller shop UI: a control character in the tagline is refused
- **WHEN** the tagline contains a newline or a NUL character and the seller submits
- **THEN** the form shows "Tagline không được chứa ký tự điều khiển." and the action is not called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/shopBranding.test.ts` `tagline control characters`

#### Scenario: Seller shop UI: an empty tagline clears the tagline only
- **WHEN** the seller empties the tagline of a storefront with a banner and featured ids and saves
- **THEN** `UpsertStorefront` is called with `tagline` empty and the banner, featured ids and theme unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `empty tagline keeps other fields`

#### Scenario: Seller shop UI: a server rejection is shown and the input kept
- **WHEN** the action receives `INVALID_ARGUMENT` from `UpsertStorefront`
- **THEN** the form shows an error alert with the server message, the typed tagline and the chosen featured listings stay in the form, and no success toast appears
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopProfileForm.test.tsx` `server rejection keeps the input`

### Requirement: The banner is uploaded through the listing image bucket

Choosing a banner file SHALL, before any request, accept only `image/jpeg`, `image/png` or `image/webp` of at most 10 MB. An accepted file SHALL be uploaded by `getUploadUrlAction` (`GetImageUploadUrl`) and a browser PUT to the returned `uploadUrl`, and the returned `publicUrl` SHALL become the banner value sent on Save. The seller SHALL be able to remove the banner, which sends an empty `banner_url`. A failed upload SHALL leave the previous banner in place and show an error line.

#### Scenario: Seller shop UI: an accepted banner is uploaded and previewed
- **WHEN** the seller picks a 2 MB `image/png` file
- **THEN** `getUploadUrlAction` is called once with `image/png`, the file is PUT once to the returned `uploadUrl`, and the preview shows the returned `publicUrl`
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopBannerField.test.tsx` `uploads and previews`

#### Scenario: Seller shop UI: a wrong type or oversize banner is refused before upload
- **WHEN** the seller picks an `application/pdf` file, then an `image/png` of 10 MB + 1 byte
- **THEN** each shows an error line ("Chỉ chấp nhận ảnh JPEG, PNG hoặc WebP." and "Kích thước file không vượt quá 10MB." respectively) and `getUploadUrlAction` is called zero times
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopBannerField.test.tsx` `type and size refused`

#### Scenario: Seller shop UI: a file of exactly 10 MB is accepted
- **WHEN** the seller picks an `image/webp` file of exactly 10 MB
- **THEN** `getUploadUrlAction` is called once
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopBannerField.test.tsx` `10 MB boundary accepted`

#### Scenario: Seller shop UI: a failed upload keeps the previous banner
- **WHEN** the presigned PUT answers HTTP 500 for a seller who already has banner B
- **THEN** the preview still shows B, an error line beginning "Upload thất bại" appears and Save still sends B
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ShopBannerField.test.tsx` `failed upload keeps previous banner`

#### Scenario: Seller shop UI: removing the banner clears it
- **WHEN** the seller clicks "Xóa banner" and saves
- **THEN** `UpsertStorefront` is called with an empty `bannerUrl` and the other fields unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `remove banner sends empty url`

### Requirement: Featured listings are picked through a server-filtered search

The picker SHALL search the seller's own listings through `ListMyListings` with `statuses = [PUBLISHED]`, the typed `query` (trimmed, <= 100 characters) and a page size of 20, debounced 300 ms, and SHALL NOT walk more than one page per search. It SHALL allow at most 6 selected listings, show them in order with move-up, move-down and remove controls, and disable the add control at 6 with the line "Tối đa 6 sản phẩm nổi bật.". Search strings SHALL NOT be logged. A featured id whose listing can no longer be loaded SHALL show as "Không còn khả dụng" with a remove control and SHALL still be removable before Save.

#### Scenario: Seller shop UI: search runs on the server with the typed query
- **WHEN** the seller types "áo" in the picker and 300 ms pass
- **THEN** exactly one `ListMyListings` call is made with `statuses = [PUBLISHED]`, `query = "áo"` and `pageSize = 20`, and no `ListMyListings` call carries a non-empty cursor
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `search is one filtered call`

#### Scenario: Seller shop UI: the seventh featured listing is refused
- **WHEN** 6 listings are selected and the seller tries to add another
- **THEN** the add control is disabled, the line "Tối đa 6 sản phẩm nổi bật." is shown and the selection stays at 6
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `cap of six`

#### Scenario: Seller shop UI: the sixth featured listing is accepted
- **WHEN** 5 listings are selected and the seller adds one more
- **THEN** 6 are selected and Save sends 6 ids in the shown order
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `sixth accepted`

#### Scenario: Seller shop UI: reordering changes the saved order
- **WHEN** the seller selects a, b, c and moves c up once, then saves
- **THEN** `UpsertStorefront` receives `featuredListingIds` equal to [a, c, b]
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `featured order is kept`

#### Scenario: Seller shop UI: an already selected listing is not added twice
- **WHEN** listing a is selected and the seller adds a again
- **THEN** the selection still contains a once
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `no duplicates`

#### Scenario: Seller shop UI: a search failure leaves the selection intact
- **WHEN** `ListMyListings` answers UNAVAILABLE during a search
- **THEN** the picker shows "Không tải được danh sách sản phẩm." with a retry button and the current selection is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `search failure keeps selection`

#### Scenario: Seller shop UI: an unavailable featured listing can be removed
- **WHEN** the storefront holds featured id x whose listing is no longer found
- **THEN** the picker shows "Không còn khả dụng" for x with a remove control, and after removing x and saving, `UpsertStorefront` receives the remaining ids without x
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/FeaturedListingsPicker.test.tsx` `unavailable featured can be removed`

#### Scenario: Seller shop UI: a featured listing of another seller is rejected by the server
- **WHEN** the action sends a featured id that belongs to another seller and `UpsertStorefront` answers INVALID_ARGUMENT
- **THEN** the form shows the error alert, the storefront returned by the next `GetStorefront` is unchanged, and the buyer shop page shows no listing of the other seller
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_shop_customisation_ui.feature` `Seller shop UI: a foreign featured listing is rejected`
