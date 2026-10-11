import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AnalyticsProvider } from "./AnalyticsProvider";

const gtmScripts = () =>
  Array.from(document.querySelectorAll("script")).filter((s) =>
    s.src.includes("googletagmanager.com"),
  );

describe("AnalyticsProvider GTM gating", () => {
  beforeEach(() => {
    document.head.innerHTML = "";
  });
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("[No GTM when ID is empty] mounting adds no googletagmanager.com script", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "");
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[GTM loads when ID is set] mounting adds exactly 1 script with GTM-TEST in src", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts().filter((s) => s.src.includes("GTM-TEST"))).toHaveLength(
      1,
    );
  });
});
