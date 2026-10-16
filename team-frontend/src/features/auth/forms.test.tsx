import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import type { AuthState } from "./actions";

// Drive the forms through controllable stand-ins for the form-action hooks.
const mocked = vi.hoisted(() => ({
  state: { ok: false } as {
    ok: boolean;
    error?: string;
    fields?: object;
    username?: string;
  },
  pending: false,
}));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useActionState: (_action: unknown, _initial: AuthState) => [
    mocked.state,
    vi.fn(),
  ],
}));
vi.mock("react-dom", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-dom")>()),
  useFormStatus: () => ({ pending: mocked.pending }),
}));
vi.mock("./actions", () => ({ loginAction: vi.fn(), registerAction: vi.fn() }));

import { LoginForm } from "./LoginForm";
import { RegisterForm } from "./RegisterForm";

beforeEach(() => {
  mocked.state = { ok: false };
  mocked.pending = false;
});

function submit(): boolean {
  const form = document.querySelector("form");
  if (!form) throw new Error("no form");
  // fireEvent returns false when a handler called preventDefault().
  return fireEvent.submit(form);
}

describe("LoginForm", () => {
  it("blocks an empty username, shows help, marks it invalid and focuses it", () => {
    render(<LoginForm />);
    const username = screen.getByLabelText(/Tên đăng nhập/);
    expect(submit()).toBe(false);
    expect(screen.getByText("Vui lòng nhập tên đăng nhập.")).toBeVisible();
    expect(username).toHaveAttribute("aria-invalid", "true");
    expect(username).toHaveFocus();
  });

  it("lets a filled form through (the server action runs)", async () => {
    const user = setupUser();
    render(<LoginForm />);
    await user.type(screen.getByLabelText(/Tên đăng nhập/), "alice");
    await user.type(screen.getByLabelText(/Mật khẩu/), "secret");
    // React 19 always preventDefaults a form whose action is a function (it
    // runs the action instead), so assert on validation feedback, not the
    // event's default-prevented flag.
    submit();
    expect(screen.queryByText("Vui lòng nhập mật khẩu.")).toBeNull();
    expect(screen.queryByText("Vui lòng nhập tên đăng nhập.")).toBeNull();
  });

  it("is pending, busy and inert while the action runs", () => {
    mocked.pending = true;
    render(<LoginForm />);
    const button = screen.getByRole("button", { name: "Đăng nhập" });
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
  });

  it("shows wrong credentials in an alert and keeps the form on the page", () => {
    mocked.state = {
      ok: false,
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
    };
    render(<LoginForm />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(
      "Tên đăng nhập hoặc mật khẩu không chính xác.",
    );
  });

  it("keeps the typed username and clears the password after a failure", async () => {
    const user = setupUser();
    const { rerender } = render(<LoginForm />);
    await user.type(screen.getByLabelText(/Tên đăng nhập/), "alice");
    await user.type(screen.getByLabelText(/Mật khẩu/), "wrong");

    mocked.state = {
      ok: false,
      error: "Tên đăng nhập hoặc mật khẩu không chính xác.",
      username: "alice",
    };
    rerender(<LoginForm />);

    expect(screen.getByLabelText(/Tên đăng nhập/)).toHaveValue("alice");
    expect(screen.getByLabelText(/Mật khẩu/)).toHaveValue("");
  });

  it("restores the submitted username even if the form was reset", () => {
    const { rerender } = render(<LoginForm />);
    const username = screen.getByLabelText(/Tên đăng nhập/) as HTMLInputElement;
    username.value = "";
    mocked.state = { ok: false, error: "Sai.", username: "bob" };
    rerender(<LoginForm />);
    expect(username).toHaveValue("bob");
  });

  it("keeps a username typed before hydration (the input is uncontrolled)", () => {
    render(<LoginForm />);
    const username = screen.getByLabelText(/Tên đăng nhập/);
    expect(username).not.toHaveAttribute("value");
  });

  it("preserves the field names and links to register", () => {
    render(<LoginForm />);
    expect(screen.getByLabelText(/Tên đăng nhập/)).toHaveAttribute(
      "name",
      "username",
    );
    expect(screen.getByLabelText(/Mật khẩu/)).toHaveAttribute(
      "name",
      "password",
    );
    expect(screen.getByRole("link", { name: "Đăng ký ngay" })).toHaveAttribute(
      "href",
      "/register",
    );
  });
});

describe("RegisterForm", () => {
  it("enforces the minimum lengths before submit", async () => {
    const user = setupUser();
    render(<RegisterForm />);
    await user.type(screen.getByLabelText(/Tên đăng nhập/), "ab");
    await user.type(screen.getByLabelText(/Mật khẩu/), "123");
    expect(submit()).toBe(false);
    expect(screen.getByText("Tên đăng nhập tối thiểu 3 ký tự.")).toBeVisible();
    expect(screen.getByText("Mật khẩu tối thiểu 4 ký tự.")).toBeVisible();
    expect(screen.getByLabelText(/Tên đăng nhập/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("keeps role, button text and the field names e2e relies on", () => {
    render(<RegisterForm />);
    expect(screen.getByLabelText("Loại tài khoản")).toHaveAttribute(
      "name",
      "role",
    );
    expect(screen.getByRole("button", { name: "Đăng ký" })).toBeInTheDocument();
  });

  it("shows a taken username against the field", () => {
    mocked.state = {
      ok: false,
      error: "Tên đăng nhập đã tồn tại.",
      fields: { username: "Tên đăng nhập đã tồn tại." },
    };
    render(<RegisterForm />);
    expect(screen.getByText("Tên đăng nhập đã tồn tại.")).toBeVisible();
    expect(screen.getByLabelText(/Tên đăng nhập/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });
});

describe("forms keep returnUrl on cross links (frontend-login-return)", () => {
  const RU = "/listing/abc?variant=v2";

  it("forms keep returnUrl on cross links: login form submits it and links to register with it", () => {
    render(<LoginForm returnUrl={RU} />);
    const hidden = document.querySelector(
      'form input[name="returnUrl"]',
    ) as HTMLInputElement | null;
    expect(hidden).not.toBeNull();
    expect(hidden?.type).toBe("hidden");
    expect(hidden?.value).toBe(RU);
    expect(screen.getByRole("link", { name: "Đăng ký ngay" })).toHaveAttribute(
      "href",
      `/register?returnUrl=${encodeURIComponent(RU)}`,
    );
  });

  it("forms keep returnUrl on cross links: register form submits it and links to login with it", () => {
    render(<RegisterForm returnUrl={RU} />);
    const hidden = document.querySelector(
      'form input[name="returnUrl"]',
    ) as HTMLInputElement | null;
    expect(hidden).not.toBeNull();
    expect(hidden?.type).toBe("hidden");
    expect(hidden?.value).toBe(RU);
    expect(
      screen.getByRole("link", { name: "Đăng nhập ngay" }),
    ).toHaveAttribute("href", `/login?returnUrl=${encodeURIComponent(RU)}`);
  });

  it("forms without returnUrl keep plain cross links and submit no target", () => {
    const { unmount } = render(<LoginForm />);
    expect(screen.getByRole("link", { name: "Đăng ký ngay" })).toHaveAttribute(
      "href",
      "/register",
    );
    const h = document.querySelector('form input[name="returnUrl"]');
    expect((h as HTMLInputElement | null)?.value ?? "").toBe("");
    unmount();
    render(<RegisterForm />);
    expect(
      screen.getByRole("link", { name: "Đăng nhập ngay" }),
    ).toHaveAttribute("href", "/login");
  });
});
