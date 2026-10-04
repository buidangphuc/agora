"use client";

import Link from "next/link";
import { useFormState } from "react-dom";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { AuthSubmitButton } from "./AuthSubmitButton";
import { type AuthState, registerAction } from "./actions";
import { useCredentialForm } from "./useCredentialForm";
import { MIN_PASSWORD, MIN_USERNAME, validateRegister } from "./validation";

const initial: AuthState = { ok: false };

const ROLE_OPTIONS = [
  { value: "buyer", label: "Người mua (buyer)" },
  { value: "seller", label: "Người bán (seller)" },
];

export function RegisterForm() {
  const [state, action] = useFormState(registerAction, initial);
  const form = useCredentialForm(state, validateRegister);

  return (
    <Card className="mx-auto w-full max-w-sm">
      <CardContent className="space-y-5">
        <div className="space-y-1 text-center">
          <h2 className="text-xl font-bold text-text-primary">
            Tạo tài khoản mới
          </h2>
          <p className="text-sm text-text-secondary">
            Chỉ mất một phút để bắt đầu.
          </p>
        </div>

        <form
          action={action}
          onSubmit={form.onSubmit}
          noValidate
          className="space-y-4"
        >
          <FormItem
            label="Tên đăng nhập"
            required
            help={form.errors.username ?? `Tối thiểu ${MIN_USERNAME} ký tự.`}
            status={form.errors.username ? "error" : undefined}
          >
            <Input
              className="min-h-10"
              ref={form.usernameRef}
              id="username"
              name="username"
              required
              autoComplete="username"
              placeholder="Chọn tên đăng nhập"
              value={form.username}
              onChange={(e) => {
                form.setUsername(e.target.value);
                form.onChange("username");
              }}
              onBlur={() => form.onBlur("username")}
            />
          </FormItem>

          <FormItem
            label="Mật khẩu"
            required
            help={form.errors.password ?? `Tối thiểu ${MIN_PASSWORD} ký tự.`}
            status={form.errors.password ? "error" : undefined}
          >
            <Input
              className="min-h-10"
              ref={form.passwordRef}
              id="password"
              name="password"
              type="password"
              required
              autoComplete="new-password"
              placeholder="Chọn mật khẩu"
              onChange={() => form.onChange("password")}
              onBlur={() => form.onBlur("password")}
            />
          </FormItem>

          <FormItem label="Loại tài khoản">
            <Select
              className="min-h-10"
              id="role"
              name="role"
              defaultValue="buyer"
              options={ROLE_OPTIONS}
            />
          </FormItem>

          {state.error && !form.hasFieldErrors && (
            <Alert type="error" description={state.error} />
          )}

          <AuthSubmitButton>Đăng ký</AuthSubmitButton>

          <p className="text-center text-sm text-text-secondary">
            Đã có tài khoản?{" "}
            <Link
              href="/login"
              className="font-medium text-action-primary hover:underline"
            >
              Đăng nhập ngay
            </Link>
          </p>
        </form>
      </CardContent>
    </Card>
  );
}
