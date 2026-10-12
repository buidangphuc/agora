import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { initGTM } from "./gtm";

const gtmScripts = () =>
  Array.from(document.querySelectorAll("script")).filter((s) =>
    s.src.includes("googletagmanager.com"),
  );

describe("initGTM", () => {
  beforeEach(() => {
    document.head.innerHTML = "";
    document.body.innerHTML = "";
    delete (window as { dataLayer?: unknown }).dataLayer;
  });
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it.each(["", "   "])(
    "[No GTM when ID is empty] NEXT_PUBLIC_GTM_ID=%j adds no googletagmanager.com script",
    (id) => {
      vi.stubEnv("NEXT_PUBLIC_GTM_ID", id);
      initGTM();
      expect(gtmScripts()).toHaveLength(0);
    },
  );

  it("[No GTM when ID is empty] NEXT_PUBLIC_GTM_ID unset adds no script", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", undefined as unknown as string);
    initGTM();
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[GTM loads when ID is set] exactly 1 script with GTM-TEST in its src", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
    initGTM();
    const withId = gtmScripts().filter((s) => s.src.includes("GTM-TEST"));
    expect(withId).toHaveLength(1);
    expect(gtmScripts()).toHaveLength(1);
  });
});
