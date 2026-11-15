import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { LEGAL_SLUGS, readLegalPage } from "./content";

const ROOT = join(__dirname, "../../..");
const SEVEN = [
  "dieu-khoan-su-dung",
  "chinh-sach-bao-mat",
  "quy-che-hoat-dong",
  "chinh-sach-doi-tra",
  "hang-cam-kinh-doanh",
  "giai-quyet-khieu-nai",
  "dieu-khoan-nguoi-ban",
];

function dockerfileDefault(name: string): string | undefined {
  const m = readFileSync(join(ROOT, "Dockerfile"), "utf8").match(
    new RegExp(`^ARG ${name}=(.*)$`, "m"),
  );
  return m ? m[1].replace(/^"|"$/g, "") : undefined;
}

function configured(name: string): string | undefined {
  return process.env[name] || dockerfileDefault(name);
}

describe("legal content", () => {
  it("[All seven slugs resolve] exposes exactly the seven slugs", () => {
    expect([...LEGAL_SLUGS].sort()).toEqual([...SEVEN].sort());
  });

  it.each(SEVEN)(
    "[All seven slugs resolve] %s has a content file and a readable page",
    (slug) => {
      expect(existsSync(join(ROOT, "content/legal", `${slug}.md`))).toBe(true);
      const page = readLegalPage(slug);
      expect(page).not.toBeNull();
      expect(page?.title.length).toBeGreaterThan(0);
      expect(page?.version.length).toBeGreaterThan(0);
    },
  );

  it.each(SEVEN)(
    "[A legal page renders title, version and placeholder] %s body is the placeholder",
    (slug) => {
      expect(readLegalPage(slug)?.body).toContain(
        "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP",
      );
    },
  );

  it("[An unknown legal slug is not found] reader returns null", () => {
    expect(readLegalPage("chinh-sach-khong-ton-tai")).toBeNull();
    expect(readLegalPage("../package")).toBeNull();
  });

  it("[Terms and privacy page versions equal the configured versions] terms", () => {
    const v = configured("NEXT_PUBLIC_LEGAL_TERMS_VERSION");
    expect(v).toBeTruthy();
    expect(readLegalPage("dieu-khoan-su-dung")?.version).toBe(v);
  });

  it("[Terms and privacy page versions equal the configured versions] privacy", () => {
    const v = configured("NEXT_PUBLIC_PRIVACY_POLICY_VERSION");
    expect(v).toBeTruthy();
    expect(readLegalPage("chinh-sach-bao-mat")?.version).toBe(v);
  });

  it("[Boundary version length] page versions are 1 to 32 characters", () => {
    for (const slug of ["dieu-khoan-su-dung", "chinh-sach-bao-mat"]) {
      const len = readLegalPage(slug)?.version.length ?? 0;
      expect(len).toBeGreaterThanOrEqual(1);
      expect(len).toBeLessThanOrEqual(32);
    }
  });

  it("[Boundary version length] Dockerfile declares both build args with defaults of 1-32 chars", () => {
    for (const n of [
      "NEXT_PUBLIC_LEGAL_TERMS_VERSION",
      "NEXT_PUBLIC_PRIVACY_POLICY_VERSION",
    ]) {
      const d = dockerfileDefault(n);
      expect(d, n).toBeDefined();
      expect(d?.length).toBeGreaterThanOrEqual(1);
      expect(d?.length).toBeLessThanOrEqual(32);
    }
  });

  it("[Terms and privacy page versions equal the configured versions] README documents both env vars", () => {
    const readme = readFileSync(join(ROOT, "README.md"), "utf8");
    expect(readme).toContain("NEXT_PUBLIC_LEGAL_TERMS_VERSION");
    expect(readme).toContain("NEXT_PUBLIC_PRIVACY_POLICY_VERSION");
  });
});
