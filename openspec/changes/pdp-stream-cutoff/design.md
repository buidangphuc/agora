## Context

`e2e-ui-rebaseline` (archived 2026-11-05) recorded the symptom and the throw site. This change finds the cause and
fixes it.

## Diagnosis

- Reproduced on the `mainx` stack and on a host `next start` of `origin/main`: one seller, listings titled
  "moderation ui listing" in `cat-electronics`, descriptions of n `x` characters; n = 7..18 cut at about 121 KB
  (curl exit 18), the rest complete at about 131 KB.
- In a cut response the AI summary boundary in `#reviews` is written as pending with no fallback markup:
  `<!--$?--><template id="B:2"></template><template id="P:3"></template><!--/$-->`. The fallback segment was still
  suspended (a Flight chunk not yet resolved), so only its placeholder went out.
- Logging inside the vendored `flushSegment` showed the throw comes from that same boundary (`rootSegmentID` 2,
  already assigned) reaching the "previously unvisited" branch with `completedSegments` empty: the segment holding it
  was flushed again and the boundary with it.
- This is facebook/react#34668. facebook/react#34694 fixes it by clearing `segment.boundary` once the boundary is
  written, and adds the regression case "not error when a suspended fallback segment directly inside another
  Suspense is abandoned". That case throws the same invariant on Next 15.5.28's vendored `react-dom/server`.
- Returning `<div hidden />` instead of `null` from `AiReviewSummary` (the interim idea in `e2e-ui-rebaseline`) does
  not help: n = 7..18 still fail.

## Decisions

- Backport the upstream line with a build-time patch script, like `patch-react-dom-ping.mjs`, instead of moving to
  Next 16 (chosen by the user 2026-11-05). Both were tried: the patched 15.5.28 and an unpatched 16.4.5 build each
  pass the 1-60 (16.4.5: 1-250) sweep with no React error in the log. Next 15.5.29 vendors the same React and still
  fails.
- The script patches every vendored Fizz bundle, not only `app-page.runtime.prod.js`, so dev, turbo and
  experimental runtimes behave the same.
- Regression coverage: a vitest of the upstream case (runs the script first, since `npm ci` installs an unpatched
  Next) and an e2e sweep of description lengths 1-40 that asserts a complete document.

## Risks

- A Next upgrade changes the bundles: the script then exits 1 and the build stops, which is the signal to drop it
  once the vendored React has the fix (Next 16 does).
- The seller and admin apps vendor the same React and can hit the same bug on their own pages; out of scope here.
