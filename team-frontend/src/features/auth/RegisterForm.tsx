"use client";

import Link from "next/link";
import { useFormState, useFormStatus } from "react-dom";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { type AuthState, registerAction } from "./actions";

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
      {pending ? "Đang tạo tài khoản..." : "Đăng ký"}
    </Button>
  );
}

export function RegisterForm() {
  const [state, action] = useFormState(registerAction, initial);
  return (
    <div className="mx-auto max-w-sm bg-white p-7 rounded-2xl border border-gray-200/90 shadow-preline-card">
      <div className="mb-6 text-center">
        <h2 className="text-xl font-bold text-gray-900">Tạo Tài Khoản</h2>
        <p className="text-xs text-gray-500 mt-1">
          Gia nhập nền tảng Marketplace Polyrepo
        </p>
      </div>

      <form action={action} className="space-y-4">
        <Input
          label="Tên đăng nhập"
          id="username"
          name="username"
          required
          minLength={3}
          autoComplete="username"
          placeholder="Tối thiểu 3 ký tự"
        />

        <Input
          label="Mật khẩu"
          id="password"
          name="password"
          type="password"
          required
          minLength={4}
          autoComplete="new-password"
          placeholder="Tối thiểu 4 ký tự"
        />

        <div className="space-y-1.5">
          <label
            htmlFor="role"
            className="block text-xs font-medium text-gray-700"
          >
            Loại tài khoản
          </label>
          <select
            id="role"
            name="role"
            defaultValue="buyer"
            className="block w-full border border-gray-200 bg-white py-2 px-3.5 text-sm rounded-lg text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary-100 focus:border-primary-500 transition"
          >
            <option value="buyer">Người mua (buyer)</option>
            <option value="seller">Người bán (seller)</option>
          </select>
        </div>

        {state.error && (
          <div className="rounded-lg bg-red-50 p-2.5 text-xs text-red-600 font-medium border border-red-200">
            {state.error}
          </div>
        )}

        <div className="pt-1">
          <SubmitButton />
        </div>

        <p className="text-center text-xs text-gray-500 pt-2">
          Đã có tài khoản?{" "}
          <Link
            href="/login"
            className="text-primary-600 font-semibold hover:underline"
          >
            Đăng nhập ngay
          </Link>
        </p>
      </form>
    </div>
  );
}
