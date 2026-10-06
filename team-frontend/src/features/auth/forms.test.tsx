import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import type { AuthState } from "./actions";

// React 18's react-dom has no form-action hooks (Next bundles a canary React
// at runtime), so the tests drive the forms through controllable stand-ins.
const mocked = vi.hoisted(() => ({
  state: { ok: false } as {
    ok: boolean;
    error?: string;
    fields?: object;
    username?: string;
  },
  pending: false,
}));

vi.mock("react-dom", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-dom")>()),
  useFormState: (_action: unknown, _initial: AuthState) => [
    mocked.state,
    vi.fn(),
  ],
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
    expect(submit()).toBe(true);
    expect(screen.queryByText("Vui lòng nhập mật khẩu.")).toBeNull();
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
