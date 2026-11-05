import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { join } from "node:path";
import { Writable } from "node:stream";

import { beforeAll, describe, expect, it } from "vitest";

const appRoot = process.cwd();
const require = createRequire(join(appRoot, "package.json"));

type ReactModule = typeof import("react");

function renderAbandonedFallback(): Promise<{
  html: string;
  errors: string[];
}> {
  const React: ReactModule = require("next/dist/compiled/react");
  const {
    renderToPipeableStream,
  } = require("next/dist/compiled/react-dom/server.node");
  const h = React.createElement;

  function SuspendForever(): null {
    React.use(new Promise<never>(() => {}));
    return null;
  }
  let resolve: (v: string) => void = () => {};
  const late = new Promise<string>((r) => {
    resolve = r;
  });
  function Suspend() {
    return React.use(late);
  }
  const app = h(
    "html",
    null,
    h(
      "body",
      null,
      h(
        React.Suspense,
        { fallback: "outer" },
        h(
          React.Suspense,
          { fallback: h(SuspendForever) },
          h("span", null, "hello world"),
          h("span", null, h(Suspend)),
        ),
      ),
    ),
  );

  return new Promise((done) => {
    let html = "";
    const errors: string[] = [];
    const sink = new Writable({
      write(chunk, _enc, cb) {
        html += chunk;
        cb();
      },
    });
    const stream = renderToPipeableStream(app, {
      onError(e: unknown) {
        errors.push(e instanceof Error ? e.message : String(e));
      },
    });
    stream.pipe(sink);
    setTimeout(() => resolve("late-content"), 20);
    setTimeout(() => {
      stream.abort();
      done({ html, errors });
    }, 300);
  });
}

describe("vendored react-dom boundary flush patch", () => {
  beforeAll(() => {
    execFileSync("node", ["scripts/patch-react-dom-boundary.mjs"], {
      cwd: appRoot,
      stdio: "pipe",
    });
  });

  it("streams a boundary whose suspended fallback is abandoned without the React invariant", async () => {
    const { html, errors } = await renderAbandonedFallback();
    expect(errors).toEqual([]);
    expect(html).toContain("late-content");
  });

  it("patches every vendored Fizz bundle", () => {
    const out = execFileSync("node", ["scripts/patch-react-dom-boundary.mjs"], {
      cwd: appRoot,
      encoding: "utf8",
    });
    const lines = out.trim().split("\n");
    expect(lines.some((l) => l.includes("app-page.runtime.prod.js"))).toBe(
      true,
    );
    expect(lines.every((l) => l.includes(": already patched "))).toBe(true);
  });
});
