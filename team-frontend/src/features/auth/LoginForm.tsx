"use client";

import { useActionState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
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
              <Input
                label="Tên đăng nhập"
                error={form.errors.username}
                ref={form.usernameRef}
                id="username"
                name="username"
                required
                autoComplete="username"
                placeholder="Nhập username của bạn"
                onChange={() => form.onChange("username")}
                onBlur={() => form.onBlur("username")}
              />

              <Input
                label="Mật khẩu"
                error={form.errors.password}
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

              {state.error && !form.hasFieldErrors && (
                <Alert type="error" description={state.error} />
              )}

              <AuthSubmitButton>Đăng nhập</AuthSubmitButton>

              <p className="text-center text-sm text-text-secondary">
                Chưa có tài khoản?{" "}
                <span className="font-medium">
                  <Link href={registerPath(returnUrl)}>Đăng ký ngay</Link>
                </span>
              </p>
            </form>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
