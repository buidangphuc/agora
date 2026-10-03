"use client";

import Link from "next/link";
import { useFormState, useFormStatus } from "react-dom";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { type AuthState, loginAction } from "./actions";

const initial: AuthState = {};

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <Button
      type="submit"
      variant="primary"
      size="md"
      isLoading={pending}
      className="w-full font-semibold shadow-sm"
    >
      {pending ? "Đang đăng nhập..." : "Đăng nhập"}
    </Button>
  );
}

export function LoginForm() {
  const [state, action] = useFormState(loginAction, initial);
  return (
    <div className="mx-auto max-w-sm bg-white p-7 rounded-2xl border border-gray-200/90 shadow-preline-card">
      <div className="mb-6 text-center">
        <h2 className="text-xl font-bold text-gray-900">Đăng Nhập</h2>
        <p className="text-xs text-gray-500 mt-1">
          Truy cập tài khoản Marketplace Polyrepo
        </p>
      </div>

      <form action={action} className="space-y-4">
        <Input
          label="Tên đăng nhập"
          id="username"
          name="username"
          required
          autoComplete="username"
          placeholder="Nhập username của bạn"
        />

        <Input
          label="Mật khẩu"
          id="password"
          name="password"
          type="password"
          required
          autoComplete="current-password"
          placeholder="••••••••"
        />

        {state.error && (
          <div className="rounded-lg bg-red-50 p-2.5 text-xs text-red-600 font-medium border border-red-200">
            {state.error}
          </div>
        )}

        <div className="pt-1">
          <SubmitButton />
        </div>

        <p className="text-center text-xs text-gray-500 pt-2">
          Chưa có tài khoản?{" "}
          <Link
            href="/register"
            className="text-primary-600 font-semibold hover:underline"
          >
            Đăng ký ngay
          </Link>
        </p>
      </form>
    </div>
  );
}
