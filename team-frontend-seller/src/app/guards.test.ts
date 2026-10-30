import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...sourceFiles(full));
    else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name))
      out.push(full);
  }
  return out;
}

// redirect("/login..."), redirect('/login...'), redirect(`/login...`)
const BARE_LOGIN_REDIRECT = /\bredirect\(\s*(["'`])\/login/;

describe("login guards", () => {
  it("[Every guard uses the helper] no bare redirect to login", () => {
    const offenders: string[] = [];
    for (const file of sourceFiles(APP_DIR)) {
      const lines = readFileSync(file, "utf8").split("\n");
      lines.forEach((line, i) => {
        if (BARE_LOGIN_REDIRECT.test(line))
          offenders.push(`${file.slice(APP_DIR.length)}:${i + 1}`);
      });
    }
    expect(offenders).toEqual([]);
  });

  it("[Every guard uses the helper] the scan pattern catches double, single and template forms", () => {
    expect(BARE_LOGIN_REDIRECT.test('redirect("/login")')).toBe(true);
    expect(BARE_LOGIN_REDIRECT.test("redirect('/login')")).toBe(true);
    expect(BARE_LOGIN_REDIRECT.test("redirect(`/login?returnUrl=${x}`)")).toBe(
      true,
    );
    expect(BARE_LOGIN_REDIRECT.test('redirect("/login?returnUrl=/chat")')).toBe(
      true,
    );
    expect(BARE_LOGIN_REDIRECT.test("redirectToLogin(path)")).toBe(false);
    expect(BARE_LOGIN_REDIRECT.test('redirect("/")')).toBe(false);
  });
});
