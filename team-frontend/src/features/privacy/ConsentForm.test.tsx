import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
  save: vi.fn(),
  toastSuccess: vi.fn(),
  refresh: vi.fn(),
}));
vi.mock("./actions", () => ({ saveConsentAction: h.save }));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({
    success: h.toastSuccess,
    error: vi.fn(),
    info: vi.fn(),
    showToast: vi.fn(),
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: h.refresh, push: vi.fn() }),
}));

import { serializeConsent } from "@/lib/consent/consent";
import { ConsentForm } from "./ConsentForm";
import { ConsentHistory } from "./ConsentHistory";

type Rec = { purpose: string; granted: boolean; version: string; source: string };
const rec = (purpose: string, granted: boolean, version = "1"): Rec => ({
  purpose,
  granted,
  version,
  source: "account",
});

function rawCookie(): string {
  const m = document.cookie.match(/(?:^|;\s*)agora_consent=([^;]*)/);
  return m ? m[1] : "";
}
function clearCookie() {
  document.cookie = "agora_consent=; Path=/; Max-Age=0";
}

beforeEach(() => {
  h.save.mockReset();
  h.toastSuccess.mockReset();
  h.refresh.mockReset();
  clearCookie();
});
afterEach(() => {
  cleanup();
  clearCookie();
});

const analytics = () => screen.getByRole("checkbox", { name: /Phân tích/i });
const marketing = () => screen.getByRole("checkbox", { name: /Tiếp thị/i });
const personalization = () =>
  screen.getByRole("checkbox", { name: /Cá nhân hóa/i });
const saveButton = () => screen.getByRole("button", { name: /Lưu lựa chọn/i });

describe("ConsentForm", () => {
  it("PV6 save is disabled until a purpose changes", async () => {
    const user = userEvent.setup();
    render(
      <ConsentForm
        records={[rec("ANALYTICS", true), rec("MARKETING", false)] as never}
      />,
    );
    expect(analytics()).toBeChecked();
    expect(marketing()).not.toBeChecked();
    expect(saveButton()).toBeDisabled();
    await user.click(saveButton());
    expect(h.save).not.toHaveBeenCalled();

    await user.click(marketing());
    expect(saveButton()).toBeEnabled();
    await user.click(marketing());
    expect(saveButton()).toBeDisabled();
    expect(h.save).not.toHaveBeenCalled();
  });

  it("PV5 saves only the changed purposes, then writes the cookie and toasts", async () => {
    const user = userEvent.setup();
    h.save.mockResolvedValue({ ok: true });
    render(<ConsentForm records={[rec("ANALYTICS", true)] as never} />);
    await user.click(analytics());
    await user.click(marketing());
    await user.click(saveButton());

    await waitFor(() => expect(h.save).toHaveBeenCalledTimes(1));
    const changes = h.save.mock.calls[0][0] as {
      purpose: string;
      granted: boolean;
    }[];
    expect(changes).toHaveLength(2);
    expect(changes).toEqual(
      expect.arrayContaining([
        { purpose: "ANALYTICS", granted: false },
        { purpose: "MARKETING", granted: true },
      ]),
    );
    await waitFor(() => expect(rawCookie()).toMatch(/a=0&m=1&p=0/));
    expect(h.toastSuccess).toHaveBeenCalledWith("Đã lưu lựa chọn của bạn.");
  });

  it("PV5 dispatches agora:consent:change after a successful save", async () => {
    const user = userEvent.setup();
    h.save.mockResolvedValue({ ok: true });
    const seen = vi.fn();
    window.addEventListener("agora:consent:change", seen);
    render(<ConsentForm records={[]} />);
    await user.click(personalization());
    await user.click(saveButton());
    await waitFor(() => expect(seen).toHaveBeenCalled());
    window.removeEventListener("agora:consent:change", seen);
  });

  it("PV7 does not write the cookie when the record fails", async () => {
    const user = userEvent.setup();
    const before = serializeConsent(
      { analytics: false, marketing: false, personalization: false },
      1760000000,
    );
    document.cookie = `agora_consent=${before}; Path=/`;
    h.save.mockResolvedValue({
      ok: false,
      error: "Chưa lưu được lựa chọn. Vui lòng thử lại.",
    });
    render(<ConsentForm records={[]} />);
    await user.click(personalization());
    await user.click(saveButton());

    expect(
      await screen.findByText("Chưa lưu được lựa chọn. Vui lòng thử lại."),
    ).toBeInTheDocument();
    expect(rawCookie()).toBe(before);
    expect(personalization()).toBeChecked();
    expect(h.toastSuccess).not.toHaveBeenCalled();
  });

  it("PV9 toggles show the cookie values when there are no records", () => {
    document.cookie = `agora_consent=${serializeConsent(
      { analytics: true, marketing: false, personalization: true },
      1760000000,
    )}; Path=/`;
    render(<ConsentForm records={[]} />);
    expect(analytics()).toBeChecked();
    expect(marketing()).not.toBeChecked();
    expect(personalization()).toBeChecked();
  });

  it("PV4 the newest record per purpose sets the toggle (records are newest first)", () => {
    render(
      <ConsentForm
        records={
          [rec("ANALYTICS", false), rec("ANALYTICS", true)] as never
        }
      />,
    );
    expect(analytics()).not.toBeChecked();
  });

  it("PV8 renders toggles only for the three banner purposes", () => {
    const records = [
      rec("TERMS", true, "2.1"),
      rec("PRIVACY_POLICY", true, "3.4"),
      rec("SELLER_TERMS", true, "5.6"),
    ] as never;
    const { container } = render(
      <>
        <ConsentForm records={records} />
        <ConsentHistory records={records} />
      </>,
    );
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);
    expect(container.textContent).toContain("2.1");
    expect(container.textContent).toContain("3.4");
    expect(container.textContent).toContain("5.6");
  });

  it("PV9 history shows the empty state", () => {
    render(<ConsentHistory records={[]} />);
    expect(screen.getByText("Chưa có lịch sử đồng ý.")).toBeInTheDocument();
  });
});
