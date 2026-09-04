"use client";

import Link from "next/link";
import { useFormState, useFormStatus } from "react-dom";

import { type AuthState, loginAction } from "./actions";

const initial: AuthState = {};
const label = "block text-sm font-medium text-gray-700";
const field =
  "mt-1 w-full rounded-md border px-3 py-2 outline-none focus:border-brand";

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending}
      className="w-full rounded-md bg-brand px-5 py-2 font-medium text-white hover:bg-brand-dark disabled:opacity-60"
    >
      {pending ? "Đang đăng nhập..." : "Đăng nhập"}
    </button>
  );
}

export function LoginForm() {
  const [state, action] = useFormState(loginAction, initial);
  return (
    <form action={action} className="mx-auto max-w-sm space-y-4">
      <div>
        <label className={label} htmlFor="username">
          Tên đăng nhập
        </label>
        <input
          id="username"
          name="username"
          required
          className={field}
          autoComplete="username"
        />
      </div>
      <div>
        <label className={label} htmlFor="password">
          Mật khẩu
        </label>
        <input
          id="password"
          name="password"
          type="password"
          required
          className={field}
          autoComplete="current-password"
        />
      </div>
      {state.error && <p className="text-red-600">{state.error}</p>}
      <SubmitButton />
      <p className="text-center text-sm text-gray-500">
        Chưa có tài khoản?{" "}
        <Link href="/register" className="text-brand hover:underline">
          Đăng ký
        </Link>
      </p>
    </form>
  );
}
