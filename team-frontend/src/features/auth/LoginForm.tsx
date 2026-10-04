"use client";

import Link from "next/link";
import { useFormState } from "react-dom";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { AuthSubmitButton } from "./AuthSubmitButton";
import { type AuthState, loginAction } from "./actions";
import { useCredentialForm } from "./useCredentialForm";
import { validateLogin } from "./validation";

const initial: AuthState = { ok: false };

export function LoginForm() {
  const [state, action] = useFormState(loginAction, initial);
  const form = useCredentialForm(state, validateLogin);

  return (
    <Card className="mx-auto w-full max-w-sm">
      <CardContent className="space-y-5">
        <div className="space-y-1 text-center">
          <h2 className="text-xl font-bold text-text-primary">
            Chào mừng trở lại
          </h2>
          <p className="text-sm text-text-secondary">
            Đăng nhập để tiếp tục mua sắm.
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
            help={form.errors.username}
            status={form.errors.username ? "error" : undefined}
          >
            <Input
              ref={form.usernameRef}
              id="username"
              name="username"
              required
              autoComplete="username"
              placeholder="Nhập username của bạn"
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
            help={form.errors.password}
            status={form.errors.password ? "error" : undefined}
          >
            <Input
              ref={form.passwordRef}
              id="password"
              name="password"
              type="password"
              required
              autoComplete="current-password"
              placeholder="••••••••"
              onChange={() => form.onChange("password")}
              onBlur={() => form.onBlur("password")}
            />
          </FormItem>

          {state.error && !form.hasFieldErrors && (
            <Alert type="error" description={state.error} />
          )}

          <AuthSubmitButton>Đăng nhập</AuthSubmitButton>

          <p className="text-center text-sm text-text-secondary">
            Chưa có tài khoản?{" "}
            <Link
              href="/register"
              className="font-medium text-action-primary hover:underline"
            >
              Đăng ký ngay
            </Link>
          </p>
        </form>
      </CardContent>
    </Card>
  );
}
