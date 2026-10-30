import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  PACKAGES,
  diffVendored,
  presentTargets,
  targets,
  write,
} from "./vendor.mjs";

const packed = new Map([
  ["a.tgz", Buffer.from("one")],
  ["b.tgz", Buffer.from("two")],
]);

function scratch() {
  return mkdtempSync(join(tmpdir(), "vendor-test-"));
}

describe("diffVendored", () => {
  it("accepts identical files", () => {
    const dir = scratch();
    try {
      write(packed, dir);
      expect(diffVendored(packed, dir)).toEqual([]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("reports a missing and a different file", () => {
    const dir = scratch();
    try {
      writeFileSync(join(dir, "a.tgz"), "other");
      const problems = diffVendored(packed, dir);
      expect(problems).toHaveLength(2);
      expect(problems[0]).toContain("a.tgz: differs");
      expect(problems[1]).toContain("b.tgz: missing");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("vendored packages", () => {
  it("lists the shared packages under stable names", () => {
    expect(PACKAGES.map((p) => p.file)).toEqual([
      "fe-platform-tokens.tgz",
      "fe-platform-ui.tgz",
      "fe-platform-api-client.tgz",
      "fe-platform-session.tgz",
    ]);
  });
});

describe("vendor targets", () => {
  it("declares the buyer, seller and admin apps", () => {
    expect(targets().map((t) => t.split("/").slice(-2).join("/"))).toEqual([
      "team-frontend/vendor",
      "team-frontend-seller/vendor",
      "team-frontend-admin/vendor",
    ]);
  });

  it("skips targets whose app directory does not exist", () => {
    const dir = scratch();
    try {
      mkdirSync(join(dir, "present"));
      const present = join(dir, "present", "vendor");
      const missing = join(dir, "missing", "vendor");
      expect(presentTargets([present, missing])).toEqual([present]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
