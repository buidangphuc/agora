import { revalidatePath } from "next/cache";
import { cookies, headers } from "next/headers";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { dismissWelcome } from "./actions";

const jar = () => cookies() as unknown as { set: ReturnType<typeof vi.fn> };

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(headers).mockReturnValue(new Headers() as never);
});

describe("dismissWelcome", () => {
  it("W14 banner sets agora_welcome_dismissed=1 with httpOnly lax path / maxAge 1209600", async () => {
    await dismissWelcome("banner");
    expect(jar().set).toHaveBeenCalledTimes(1);
    expect(jar().set).toHaveBeenCalledWith(
      "agora_welcome_dismissed",
      "1",
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 1209600,
        secure: false,
      }),
    );
  });

  it("W14 banner cookie is secure when the request is https like the session cookie", async () => {
    vi.mocked(headers).mockReturnValue(
      new Headers({ "x-forwarded-proto": "https" }) as never,
    );
    await dismissWelcome("banner");
    expect(jar().set).toHaveBeenCalledWith(
      "agora_welcome_dismissed",
      "1",
      expect.objectContaining({ secure: true }),
    );
  });

  it("W14 banner dismissal revalidates the current page", async () => {
    await dismissWelcome("banner");
    expect(revalidatePath).toHaveBeenCalled();
  });

  it("W19 checklist sets agora_checklist_dismissed=1 with the same attributes", async () => {
    await dismissWelcome("checklist");
    expect(jar().set).toHaveBeenCalledTimes(1);
    expect(jar().set).toHaveBeenCalledWith(
      "agora_checklist_dismissed",
      "1",
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 1209600,
        secure: false,
      }),
    );
    expect(revalidatePath).toHaveBeenCalled();
  });

  it("W19 dismissing the checklist leaves the banner cookie untouched", async () => {
    await dismissWelcome("checklist");
    expect(jar().set).not.toHaveBeenCalledWith(
      "agora_welcome_dismissed",
      expect.anything(),
      expect.anything(),
    );
  });

  it("W14 an unknown part sets no cookie (boundary)", async () => {
    await dismissWelcome("nope" as never);
    expect(jar().set).not.toHaveBeenCalled();
  });
});
