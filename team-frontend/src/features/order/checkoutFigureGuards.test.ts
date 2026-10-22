import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

const roots = ["src/app/(checkout)", "src/features/order"];
const TEST_FILE = /\.test\.tsx?$/;

function sources(): { file: string; text: string }[] {
  const out: { file: string; text: string }[] = [];
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      const full = join(dir, name);
      if (statSync(full).isDirectory()) walk(full);
      else if (/\.tsx?$/.test(name) && !TEST_FILE.test(name)) {
        out.push({ file: full, text: readFileSync(full, "utf8") });
      }
    }
  };
  for (const r of roots) walk(join(process.cwd(), r));
  return out;
}

describe("Checkout coherence: checkout calls only the preview for figures (1.8)", () => {
  it("Checkout coherence: checkout calls only the preview for figures - no previewVoucher or calculateShippingFee in checkout paths", () => {
    const hits = sources()
      .filter(({ text }) => /previewVoucher|calculateShippingFee/.test(text))
      .map(({ file }) => file);
    expect(hits).toEqual([]);
  });

  it("Checkout coherence: the frontend adds nothing up - checkout paths do not import the cart voucher modal, voucher actions or orderTotal", () => {
    const hits = sources()
      .filter(({ text }) =>
        /features\/voucher\/(VoucherModal|actions)|from "\.\/orderTotal"|from "@\/features\/order\/orderTotal"/.test(
          text,
        ),
      )
      .map(({ file }) => file);
    expect(hits).toEqual([]);
  });

  it("Checkout coherence: checkout calls only the preview for figures - the summary page uses previewCheckout", () => {
    const page = sources().find(({ file }) =>
      file.endsWith("(summary)/page.tsx"),
    );
    expect(page?.text).toMatch(/previewCheckout/);
  });
});
