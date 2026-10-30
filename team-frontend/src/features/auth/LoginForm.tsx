"use client";

import Link from "next/link";
import { useActionState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { AuthSubmitButton } from "./AuthSubmitButton";
import { type AuthState, loginAction } from "./actions";
import { registerPath } from "./returnUrlShared";
import { useCredentialForm } from "./useCredentialForm";
import { validateLogin } from "./validation";

const initial: AuthState = { ok: false };

export function LoginForm({ returnUrl }: { returnUrl?: string }) {
  const [state, action] = useActionState(loginAction, initial);
  const form = useCredentialForm(state, validateLogin);

  return (
    <div className="mx-auto w-full max-w-sm">
      <Card>
        <CardContent>
          <div className="space-y-5">
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
              {returnUrl ? (
                <input type="hidden" name="returnUrl" value={returnUrl} />
              ) : null}
              <FormItem
                label="Tên đăng nhập"
                required
                help={form.errors.username}
                status={form.errors.username ? "error" : undefined}
              >
                <Input
                  className="min-h-10"
                  ref={form.usernameRef}
                  id="username"
                  name="username"
                  required
                  autoComplete="username"
                  placeholder="Nhập username của bạn"
                  onChange={() => form.onChange("username")}
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
                  className="min-h-10"
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
                  href={registerPath(returnUrl)}
                  className="font-medium text-action-primary hover:underline"
                >
                  Đăng ký ngay
                </Link>
              </p>
            </form>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
