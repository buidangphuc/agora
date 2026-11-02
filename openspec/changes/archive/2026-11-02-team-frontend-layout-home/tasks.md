## 1. Layout

- [x] 1.1 Restyle `Hero`, `ServiceHubs`, `FlashSaleSection` and the page column; verify scoped vitest and `tsc` pass
- [x] 1.2 Add `SectionHeading`, use it in `FeedBlock` and `RecentlyViewedRow`, move retry and "see more" links to button variants; verify the home block tests pass
- [x] 1.3 Match `HomeSkeletons` and `loading.tsx` to the new footprints; verify the streaming and loading tests pass

## 2. Verification

- [x] 2.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 2.2 Screenshots of `/` at 375, 768 and 1280 px with no horizontal overflow and controls at least 44 px high (gateway down, so the feed error state)
- [x] 2.3 `openspec validate team-frontend-layout-home --strict`
