import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ exportData: vi.fn() }));
vi.mock("./actions", () => ({ exportDataAction: h.exportData }));

import { ExportButton } from "./ExportButton";

const label = /Tải dữ liệu của tôi/i;
let downloads: { name: string; href: string }[];
let blobs: Blob[];

beforeEach(() => {
  h.exportData.mockReset();
  downloads = [];
  blobs = [];
  Object.assign(URL, {
    createObjectURL: vi.fn((b: Blob) => {
      blobs.push(b);
      return "blob:agora-test";
    }),
    revokeObjectURL: vi.fn(),
  });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    downloads.push({ name: this.download, href: this.href });
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ExportButton", () => {
  it("PV11 downloads one application/json Blob named agora-du-lieu-<date>.json", async () => {
    const user = userEvent.setup();
    const content = '{"account":{"user_id":"u-1"}}';
    h.exportData.mockResolvedValue({
      ok: true,
      data: { filename: "agora-du-lieu-2026-11-13.json", content },
    });
    render(<ExportButton />);
    await user.click(screen.getByRole("button", { name: label }));

    await waitFor(() => expect(downloads).toHaveLength(1));
    expect(downloads[0].name).toBe("agora-du-lieu-2026-11-13.json");
    expect(blobs).toHaveLength(1);
    expect(blobs[0].type).toBe("application/json");
    expect(await blobs[0].text()).toBe(content);
    expect(document.body.textContent).not.toContain("u-1");
  });

  it("PV12 shows an error and re-enables the button", async () => {
    const user = userEvent.setup();
    h.exportData.mockResolvedValue({
      ok: false,
      error: "Chưa tải được dữ liệu. Vui lòng thử lại sau.",
    });
    render(<ExportButton />);
    await user.click(screen.getByRole("button", { name: label }));

    expect(
      await screen.findByText("Chưa tải được dữ liệu. Vui lòng thử lại sau."),
    ).toBeInTheDocument();
    expect(downloads).toHaveLength(0);
    expect(screen.getByRole("button", { name: label })).toBeEnabled();
  });

  it("PV14 blocks a second click while pending", async () => {
    let release!: (v: unknown) => void;
    h.exportData.mockReturnValue(new Promise((r) => (release = r)));
    render(<ExportButton />);
    const btn = screen.getByRole("button", { name: label });
    await act(async () => {
      btn.click();
      btn.click();
    });
    expect(h.exportData).toHaveBeenCalledTimes(1);
    await act(async () => {
      release({
        ok: true,
        data: { filename: "agora-du-lieu-2026-11-13.json", content: "{}" },
      });
    });
    await waitFor(() => expect(downloads).toHaveLength(1));
    expect(h.exportData).toHaveBeenCalledTimes(1);
  });
});
