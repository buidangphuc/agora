import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.hoisted(() => {
  process.env.NEXT_PUBLIC_LEGAL_TERMS_VERSION = "2026-11-13";
  process.env.NEXT_PUBLIC_PRIVACY_POLICY_VERSION = "2026-11-14";
});

const m = vi.hoisted(() => ({
  pathname: "/",
  getLegalAcceptance: vi.fn(),
  acceptCurrentLegal: vi.fn(),
  logoutAction: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  usePathname: () => m.pathname,
}));
vi.mock("./actions", () => ({
  getLegalAcceptance: m.getLegalAcceptance,
  acceptCurrentLegal: m.acceptCurrentLegal,
}));
vi.mock("@/features/auth/actions", () => ({
  logoutAction: m.logoutAction,
}));

import { LegalReacceptance } from "./LegalReacceptance";

const KEY = "agora_legal_ok";
const VALUE = "2026-11-13|2026-11-14";

beforeEach(() => {
  vi.clearAllMocks();
  m.pathname = "/";
  sessionStorage.clear();
  m.acceptCurrentLegal.mockResolvedValue({ ok: true });
});
afterEach(() => cleanup());

describe("LegalReacceptance", () => {
  it("[A legacy user is prompted and accepts] shows the blocking dialog, Đồng ý records once and closes it", async () => {
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    const user = userEvent.setup();
    render(<LegalReacceptance />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Đồng ý" }));
    expect(m.acceptCurrentLegal).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(sessionStorage.getItem(KEY)).toBe(VALUE);
  });

  it("[A newer Terms version triggers the prompt] the dialog offers exactly the two actions", async () => {
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    render(<LegalReacceptance />);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.querySelectorAll("button")).toHaveLength(2);
  });

  it("[A newer Terms version triggers the prompt] legal pages stay readable: no dialog on a legal route", async () => {
    m.pathname = "/dieu-khoan-su-dung";
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    render(<LegalReacceptance />);
    await act(async () => {});
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("[Up-to-date users are not prompted] no dialog, and the satisfied check is remembered for the tab", async () => {
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: false,
    });
    render(<LegalReacceptance />);
    await waitFor(() => expect(sessionStorage.getItem(KEY)).toBe(VALUE));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("[Up-to-date users are not prompted] a remembered check skips the lookup entirely", async () => {
    sessionStorage.setItem(KEY, VALUE);
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    render(<LegalReacceptance />);
    await act(async () => {});
    expect(m.getLegalAcceptance).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("[A newer Terms version triggers the prompt] a remembered check for other versions does not suppress the prompt", async () => {
    sessionStorage.setItem(KEY, "2025-01-01|2025-01-01");
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    render(<LegalReacceptance />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("[Anonymous visitors are never prompted] signedIn:false shows no dialog", async () => {
    m.getLegalAcceptance.mockResolvedValue({ signedIn: false });
    render(<LegalReacceptance />);
    await waitFor(() => expect(m.getLegalAcceptance).toHaveBeenCalled());
    await act(async () => {});
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("[Consent lookup failure does not block browsing] a rejected action shows no dialog and no error", async () => {
    m.getLegalAcceptance.mockRejectedValue(new Error("UNAVAILABLE"));
    render(<LegalReacceptance />);
    await waitFor(() => expect(m.getLegalAcceptance).toHaveBeenCalled());
    await act(async () => {});
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("[Declining signs the user out] Đăng xuất calls logout and records nothing", async () => {
    m.getLegalAcceptance.mockResolvedValue({
      signedIn: true,
      needsAcceptance: true,
    });
    const user = userEvent.setup();
    render(<LegalReacceptance />);
    await user.click(await screen.findByRole("button", { name: "Đăng xuất" }));
    expect(m.logoutAction).toHaveBeenCalledTimes(1);
    expect(m.acceptCurrentLegal).not.toHaveBeenCalled();
  });
});
