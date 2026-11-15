import { describe, expect, it } from "vitest";

import { validateRegister } from "./validation";

const base = { username: "alice", password: "pass1234" };

describe("validateRegister acceptance", () => {
  it("[Registering with both boxes checked records two consents] no error when both are accepted", () => {
    expect(
      validateRegister({ ...base, acceptTerms: true, acceptPrivacy: true }),
    ).toEqual({});
  });

  it("[Unchecked box blocks submission] Terms unchecked yields a Vietnamese error on acceptTerms only", () => {
    const e = validateRegister({
      ...base,
      acceptTerms: false,
      acceptPrivacy: true,
    }) as Record<string, string | undefined>;
    expect(e.acceptTerms).toMatch(/đồng ý/i);
    expect(e.acceptPrivacy).toBeUndefined();
  });

  it("[Unchecked box blocks submission] Privacy unchecked yields a Vietnamese error on acceptPrivacy only", () => {
    const e = validateRegister({
      ...base,
      acceptTerms: true,
      acceptPrivacy: false,
    }) as Record<string, string | undefined>;
    expect(e.acceptPrivacy).toMatch(/đồng ý/i);
    expect(e.acceptTerms).toBeUndefined();
  });

  it("[A forged post without acceptance fields is refused] missing acceptance values count as unchecked", () => {
    const e = validateRegister({ ...base } as never) as Record<
      string,
      string | undefined
    >;
    expect(e.acceptTerms).toBeTruthy();
    expect(e.acceptPrivacy).toBeTruthy();
  });
});
