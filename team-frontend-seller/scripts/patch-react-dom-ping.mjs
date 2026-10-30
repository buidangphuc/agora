#!/usr/bin/env node
/**
 * Backports a one-line React fix into the react-dom that Next 15.5 vendors
 * (next/dist/compiled/react-dom*, 19.2.0-canary-0bdb9206-20250818).
 *
 * Bug: pingSuspendedRoot drops a ping that arrives *during* render while the
 * root is RootSuspendedWithDelay (a transition that must stay on the previous
 * screen). Next's Flight client resolves an already-received ("resolved_model")
 * chunk synchronously inside `.then`, so that ping fires mid-render, is lost,
 * and the navigation transition never commits: the RSC response arrives but the
 * URL and UI never change. It hits same-route navigations that only change
 * search params (?step=, ?tab=, ?sort=, facets), where the page's loading
 * boundary is already showing content. React 19.3 records the ping in
 * workInProgressRootPingedLanes instead; this applies the same change.
 *
 * Idempotent. Exits 1 if a file matches neither the buggy nor the fixed form,
 * so a Next upgrade forces a re-check (drop this script once Next vendors a
 * react-dom with the fix).
 */
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const compiled = join(root, "node_modules/next/dist/compiled");

const FILES = ["react-dom", "react-dom-experimental"].flatMap((pkg) =>
  [
    "react-dom-client.production.js",
    "react-dom-client.development.js",
    "react-dom-profiling.profiling.js",
    "react-dom-profiling.development.js",
  ].map((f) => join(compiled, pkg, "cjs", f)),
);

// [buggy, fixed] for the production and development builds.
const NOT_RENDERING = [
  "0 === (executionContext & 2)",
  "(executionContext & RenderContext) === NoContext",
];
const PINGED = "(workInProgressRootPingedLanes |= pingedLanes)";
const reEscape = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

let failed = false;
for (const file of FILES) {
  if (!existsSync(file)) continue;
  let src = readFileSync(file, "utf8");
  let state = "unknown";
  for (const cond of NOT_RENDERING) {
    const buggy = new RegExp(
      `\\?\\s*${reEscape(cond)}\\s*&&\\s*prepareFreshStack\\(root, 0\\)\\s*:\\s*${reEscape(PINGED)}`,
    );
    const fixed = new RegExp(
      `\\?\\s*${reEscape(cond)}\\s*\\?\\s*prepareFreshStack\\(root, 0\\)\\s*:\\s*${reEscape(PINGED)}`,
    );
    if (buggy.test(src)) {
      src = src.replace(
        buggy,
        `? ${cond} ? prepareFreshStack(root, 0) : ${PINGED} : ${PINGED}`,
      );
      writeFileSync(file, src);
      state = "patched";
      break;
    }
    if (fixed.test(src)) {
      state = "already patched";
      break;
    }
  }
  console.log(`patch-react-dom-ping: ${state} ${file.slice(root.length + 1)}`);
  if (state === "unknown") failed = true;
}
if (failed) {
  console.error(
    "patch-react-dom-ping: pingSuspendedRoot changed; re-check whether the vendored react-dom still needs this patch.",
  );
  process.exit(1);
}
