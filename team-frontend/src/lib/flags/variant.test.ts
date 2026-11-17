import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

import { OpenFeature, type Provider } from "@openfeature/server-sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./provider.js", () => ({
  registerFliptProvider: vi.fn(async () => {}),
}));

const SALT = "test-salt-123";

type Behaviour = (targetingKey: string | undefined) => Promise<string>;

function fakeFlipt(behaviour: Behaviour) {
  const calls: Array<string | undefined> = [];
  const provider = {
    metadata: { name: "fake-flipt" },
    runsOn: "server",
    async resolveStringEvaluation(
      _flag: string,
      _def: string,
      ctx: { targetingKey?: string },
    ) {
      calls.push(ctx.targetingKey);
      const value = await behaviour(ctx.targetingKey);
      return { value, variant: value };
    },
    async resolveBooleanEvaluation(_f: string, d: boolean) {
      return { value: d };
    },
    async resolveNumberEvaluation(_f: string, d: number) {
      return { value: d };
    },
    async resolveObjectEvaluation(_f: string, d: object) {
      return { value: d };
    },
  } as unknown as Provider;
  return { provider, calls };
}

async function load(env: Record<string, string> = {}) {
  vi.resetModules();
  vi.stubEnv("FLAG_ENTITY_SALT", SALT);
  vi.stubEnv("FEATURE_FLAGS_ENABLED", "true");
  for (const [k, v] of Object.entries(env)) vi.stubEnv(k, v);
  return import("./variant.js");
}

beforeEach(() => {
  vi.useRealTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

// Scenario: A resolved variant is returned and the id is hashed
describe("getVariant", () => {
  it("hashes the entity id", async () => {
    const { provider, calls } = fakeFlipt(async () => "b");
    await OpenFeature.setProviderAndWait(provider);
    const { getVariant } = await load();

    await expect(getVariant("home-feed-ranker", "u-1")).resolves.toBe("b");

    const expected = createHash("sha256").update(`${SALT}u-1`).digest("hex");
    expect(calls).toEqual([expected]);
    expect(expected).toMatch(/^[0-9a-f]{64}$/);
    expect(String(calls[0])).not.toContain("u-1");
  });

  // Scenario: A slow or failing Flipt yields no variant
  it("times out to null", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const { provider } = fakeFlipt(
      () => new Promise((resolve) => setTimeout(() => resolve("b"), 80)),
    );
    await OpenFeature.setProviderAndWait(provider);
    const { getVariant } = await load();

    vi.useFakeTimers();
    let settled = false;
    const pending = getVariant("home-feed-ranker", "u-1").then((v) => {
      settled = true;
      return v;
    });
    await vi.advanceTimersByTimeAsync(60);
    expect(settled).toBe(true);
    await expect(pending).resolves.toBeNull();

    const failing = fakeFlipt(async () => {
      throw new Error("flipt down");
    });
    vi.useRealTimers();
    await OpenFeature.setProviderAndWait(failing.provider);
    await expect(getVariant("home-feed-ranker", "u-1")).resolves.toBeNull();

    const logged = [warn, error, log]
      .flatMap((s) => s.mock.calls)
      .map((args) => args.map((a) => String(a)).join(" "))
      .join("\n");
    expect(logged).not.toContain("u-1");
  });

  // Scenario: Disabled flags or an empty entity yield no variant
  it("returns null when disabled or without entity", async () => {
    const { provider, calls } = fakeFlipt(async () => "b");
    await OpenFeature.setProviderAndWait(provider);

    const enabled = await load();
    await expect(
      enabled.getVariant("home-feed-ranker", ""),
    ).resolves.toBeNull();

    const disabled = await load({ FEATURE_FLAGS_ENABLED: "false" });
    await expect(
      disabled.getVariant("home-feed-ranker", "u-1"),
    ).resolves.toBeNull();

    expect(calls).toEqual([]);
  });
});

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name);
    return statSync(p).isDirectory() ? walk(p) : [p];
  });
}

describe("lib/flags boundary", () => {
  // Scenario: A browser bundle never contains the flag client
  it("is server-only", () => {
    const src = readFileSync(join(__dirname, "variant.ts"), "utf8");
    expect(src).toMatch(/^import ["']server-only["'];?$/m);

    const offenders = walk(join(__dirname, "..", ".."))
      .filter((f) => /\.(ts|tsx)$/.test(f) && !/\.test\./.test(f))
      .filter((f) => {
        const text = readFileSync(f, "utf8");
        return (
          /^["']use client["']/m.test(text) &&
          /from ["'](@\/lib\/flags|[./]+\/(lib\/)?flags)(\/[^"']*)?["']/.test(
            text,
          )
        );
      });
    expect(offenders).toEqual([]);
  });
});
