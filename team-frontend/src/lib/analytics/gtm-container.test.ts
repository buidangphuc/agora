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
interface Variable {
  name?: string;
  type?: string;
  parameter?: Param[];
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
    variables: (version.variable ?? []) as Variable[],
  };
}

const param = (t: Tag, key: string) =>
  t.parameter?.find((p) => p.key === key)?.value;

const triggerOf = (id: string, triggers: Trigger[]) =>
  triggers.find((tr) => String(tr.triggerId) === String(id));
const firesOnIdentity = (t: Tag, triggers: Trigger[]) =>
  (t.firingTriggerId ?? []).some((id) => {
    const tr = triggerOf(id, triggers);
    return (
      tr?.type === "CUSTOM_EVENT" &&
      JSON.stringify(tr.customEventFilter ?? []).includes("ga4_identity")
    );
  });
const ALL_PAGES_BUILTIN = "2147479553";
const firesOnAllPages = (t: Tag, triggers: Trigger[]) =>
  (t.firingTriggerId ?? []).some(
    (id) =>
      String(id) === ALL_PAGES_BUILTIN ||
      triggerOf(id, triggers)?.type === "PAGEVIEW",
  );

describe("gtm-container.json", () => {
  it("[Container defines event tags for the ecommerce funnel] has exactly 2 GA4 configuration tags: initial (All Pages) and identity", () => {
    const { tags, triggers } = load();
    const cfgs = tags.filter((t) => t.type === "gaawc");
    expect(cfgs).toHaveLength(2);
    expect(cfgs.filter((t) => firesOnIdentity(t, triggers))).toHaveLength(1);
    expect(cfgs.filter((t) => firesOnAllPages(t, triggers))).toHaveLength(1);
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

  describe.each(["user_id", "debug_mode"])("config field %s", (field) => {
    it(`[Identity and debug flags reach GA4] defines a v2 Data Layer Variable for ${field}`, () => {
      const { variables } = load();
      const dlv = variables.filter(
        (v) =>
          v.type === "v" &&
          v.parameter?.some((p) => p.key === "name" && p.value === field) &&
          v.parameter?.some(
            (p) => p.key === "dataLayerVersion" && String(p.value) === "2",
          ),
      );
      expect(dlv).toHaveLength(1);
    });

    it(`[Identity and debug flags reach GA4] the GA4 config tag maps ${field} from its Data Layer Variable`, () => {
      const { tags, variables } = load();
      const dlv = variables.find(
        (v) =>
          v.type === "v" &&
          v.parameter?.some((p) => p.key === "name" && p.value === field),
      );
      expect(dlv?.name).toBeTruthy();
      const cfgs = tags.filter((t) => t.type === "gaawc");
      expect(cfgs).toHaveLength(2);
      for (const cfg of cfgs) {
        const json = JSON.stringify(
          (cfg.parameter ?? []).filter((p) => p.key !== "measurementId"),
        );
        expect(json, cfg.name).toContain(`"${field}"`);
        expect(json, cfg.name).toContain(`{{${dlv?.name}}}`);
      }
    });
  });

  it("[Identity reaches GA4 after the async fetch] a config tag fires on the ga4_identity CUSTOM_EVENT trigger", () => {
    const { tags, triggers } = load();
    const cfgs = tags.filter(
      (t) => t.type === "gaawc" && firesOnIdentity(t, triggers),
    );
    expect(cfgs.length).toBeGreaterThan(0);
  });

  it("[Identity arrival does not duplicate page_view] every config tag firing on ga4_identity has sendPageView false and not the All Pages trigger", () => {
    const { tags, triggers } = load();
    const cfgs = tags.filter(
      (t) => t.type === "gaawc" && firesOnIdentity(t, triggers),
    );
    expect(cfgs.length).toBeGreaterThan(0);
    for (const t of cfgs) {
      expect(param(t, "sendPageView"), t.name).toBe("false");
      expect(firesOnAllPages(t, triggers), t.name).toBe(false);
    }
  });

  it("[Identity arrival does not duplicate page_view] the All Pages config tag does not fire on ga4_identity and keeps page_view on", () => {
    const { tags, triggers } = load();
    const initial = tags.filter(
      (t) => t.type === "gaawc" && firesOnAllPages(t, triggers),
    );
    expect(initial).toHaveLength(1);
    expect(firesOnIdentity(initial[0], triggers)).toBe(false);
    expect(param(initial[0], "sendPageView")).not.toBe("false");
  });

  it("[Identity arrival does not duplicate page_view] both config tags share the same measurement id variable", () => {
    const { tags } = load();
    const ids = tags
      .filter((t) => t.type === "gaawc")
      .map(
        (t) => param(t, "measurementId") ?? param(t, "measurementIdOverride"),
      );
    expect(ids).toHaveLength(2);
    expect(ids[0]).toBe(ids[1]);
  });
});
