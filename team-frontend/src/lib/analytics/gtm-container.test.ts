import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const FUNNEL = [
  "view_item_list",
  "select_item",
  "view_item",
  "add_to_cart",
  "remove_from_cart",
  "view_cart",
  "begin_checkout",
  "add_shipping_info",
  "add_payment_info",
] as const;

interface Param {
  key: string;
  value?: string;
}
interface Tag {
  name?: string;
  type?: string;
  parameter?: Param[];
  firingTriggerId?: string[];
  consentSettings?: { consentStatus?: string };
}
interface Trigger {
  triggerId?: string;
  type?: string;
  customEventFilter?: unknown[];
}

const PATH = resolve(__dirname, "../../../analytics/gtm-container.json");

function load() {
  const text = readFileSync(PATH, "utf-8");
  const data = JSON.parse(text);
  const version = data.containerVersion ?? data;
  return {
    text,
    tags: (version.tag ?? []) as Tag[],
    triggers: (version.trigger ?? []) as Trigger[],
  };
}

const param = (t: Tag, key: string) =>
  t.parameter?.find((p) => p.key === key)?.value;

describe("gtm-container.json", () => {
  it("[Container defines event tags for the ecommerce funnel] has exactly 1 GA4 configuration tag", () => {
    const { tags } = load();
    expect(tags.filter((t) => t.type === "gaawc")).toHaveLength(1);
  });

  it.each(FUNNEL)(
    "[Container defines event tags for the ecommerce funnel] %s has exactly one GA4 event tag with a same-name CUSTOM_EVENT trigger",
    (name) => {
      const { tags, triggers } = load();
      const tagged = tags.filter(
        (t) => t.type === "gaawe" && param(t, "eventName") === name,
      );
      expect(tagged).toHaveLength(1);
      const firing = (tagged[0].firingTriggerId ?? []).map((id) =>
        triggers.find((tr) => String(tr.triggerId) === String(id)),
      );
      expect(
        firing.some(
          (tr) =>
            tr?.type === "CUSTOM_EVENT" &&
            JSON.stringify(tr.customEventFilter ?? []).includes(name),
        ),
      ).toBe(true);
    },
  );

  it("[Every tag is consent-gated] every tag requires analytics_storage and none is unconditional", () => {
    const { tags } = load();
    expect(tags.length).toBeGreaterThan(0);
    for (const t of tags) {
      const cs = t.consentSettings ?? {};
      expect(String(cs.consentStatus).toUpperCase(), t.name).toBe("NEEDED");
      expect(JSON.stringify(cs), t.name).toContain("analytics_storage");
    }
  });

  it("[Purchase is not sent from the browser] no tag has event name purchase", () => {
    const { tags } = load();
    expect(tags.some((t) => t.type === "gaawe")).toBe(true);
    const purchase = tags.filter(
      (t) =>
        param(t, "eventName") === "purchase" ||
        String(t.name ?? "")
          .trim()
          .toLowerCase() === "purchase",
    );
    expect(purchase).toHaveLength(0);
  });

  it("[Container contains no key or secret values] no secret literals", () => {
    const { text } = load();
    for (const env of ["GA4_USER_ID_HMAC_KEY", "GA4_API_SECRET"]) {
      const v = process.env[env];
      if (v) expect(text).not.toContain(v);
    }
    expect(text).not.toMatch(/api_secret\s*[=:]\s*["']?[A-Za-z0-9_-]{8,}/i);
  });

  it("[Container contains no key or secret values] measurement id is a variable, not a literal", () => {
    const { text, tags } = load();
    expect(text).not.toMatch(/\bG-[A-Z0-9]{6,}\b/);
    const refs = tags
      .filter((t) => t.type === "gaawc")
      .map(
        (t) => param(t, "measurementId") ?? param(t, "measurementIdOverride"),
      );
    expect(refs.length).toBeGreaterThan(0);
    for (const r of refs) expect(r?.startsWith("{{")).toBe(true);
  });
});
