import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..");
const INTERNAL_SELLER_LINK =
  /\b(?:href|push|replace|redirect|revalidatePath)\s*[=:(]\s*\{?\s*["'`]\/sell(?:er)?(?=[/?#"'`])/;

function sources(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (name !== "generated") out.push(...sources(full));
    } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name)) {
      out.push(full);
    }
  }
  return out;
}

describe("seller pages live in the seller app", () => {
  it("has no /seller or /sell route in the buyer app", () => {
    for (const route of ["seller", "sell"]) {
      expect(existsSync(join(SRC, "app", "(shop)", route))).toBe(false);
      expect(existsSync(join(SRC, "app", route))).toBe(false);
    }
  });

  it("never links to /seller or /sell inside the buyer app", () => {
    const offenders: string[] = [];
    for (const file of sources(SRC)) {
      readFileSync(file, "utf8")
        .split("\n")
        .forEach((line, i) => {
          if (INTERNAL_SELLER_LINK.test(line))
            offenders.push(`${relative(SRC, file)}:${i + 1}`);
        });
    }
    expect(offenders).toEqual([]);
  });

  it("flags the link shapes it is meant to catch", () => {
    for (const line of [
      '<Link href="/seller">',
      "<Link href={`/seller/orders/${id}`}>",
      'redirect("/sell")',
      '{ href: "/seller/new", label: "x" }',
      'revalidatePath("/seller")',
    ]) {
      expect(INTERNAL_SELLER_LINK.test(line), line).toBe(true);
    }
    for (const line of [
      '<a href={sellerAppUrl("/seller")}>',
      '<Link href="/shop/seller-1">',
      '<Link href="/sellers-guide">',
    ]) {
      expect(INTERNAL_SELLER_LINK.test(line), line).toBe(false);
    }
  });
});
