# P5 experiment brief

Baseline before the run (commit 9664a1f9): `npm run check` exits 0 (110 tests), template build passes, `scripts/check-disables.mjs` reports 7 disable(s) with 0 violations.

## Prompt given to the agent (verbatim, nothing else)

You are working in the `fe-platform/` directory of this repository. Use only what is in this repository.

Build a five-screen shopping flow in the Next.js template (`templates/app`) under the public route group, at these routes:

1. `/shop` browse: a list of products
2. `/shop/[id]` product: one product with an add-to-cart action
3. `/shop/cart` cart: the lines, a total, and a way to continue
4. `/shop/checkout` checkout: a form to enter delivery details and place the order
5. `/shop/confirmation` confirmation: an order summary

Use static typed data in a file inside the template (no network calls, no new dependencies, no client state library; pass state through the URL or constants).

You are done when `npm run check` exits 0 and `npm run build -w templates/app` passes, with no new lint or type disable comments.

When finished, reply with a short report: what you built, which files you added, and anything that was unclear, missing, or that you had to work around.
