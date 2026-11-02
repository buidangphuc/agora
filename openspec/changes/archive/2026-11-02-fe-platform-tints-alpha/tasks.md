## 1. Tokens

- [x] 1.1 Add the primitives, the six semantic colours and the contrast pairs; `npm run build` in `packages/tokens`; verify the contrast test passes in light and dark
- [x] 1.2 Add and export `colorVar` and use it in the preset; add a test that a semantic colour with `/50` resolves to a `color-mix` with 50%; verify `dist/tokens.css` is unchanged apart from the new variables

## 2. Button

- [x] 2.1 Button loading overlay; add a ui test that the loading Button keeps its label in the DOM inside an `opacity-0` span and shows one spinner; `npm run catalog`; verify `npm run check` in `fe-platform`

## 3. Release notes

- [x] 3.1 Add changesets (tokens minor, ui patch)

## 4. Apps

- [x] 4.1 `npm run vendor` and fix the lockfile integrity in the buyer, seller and admin apps; switch each app's tailwind aliases to `colorVar`
- [x] 4.2 Remove the `w-44` wrappers in `PlaceOrderForm` (buyer) and `ListingForm` (seller)
- [x] 4.3 Apply the class mapping in the buyer app and the `tones.ts` soft tones
- [x] 4.4 Extend `palette.test.ts`; verify it fails when `bg-red-50` or `text-white/80` is added to a component and passes after reverting

## 5. Verification

- [x] 5.1 Run `npm run check` and `npm run build` in all three apps
- [x] 5.2 Serve the built buyer app. Check the computed colours of the shell top bar, the chat list, the loyalty widget and the loan calculator error. Check that a loading submit button keeps its width
- [x] 5.3 `openspec validate fe-platform-tints-alpha --strict`
