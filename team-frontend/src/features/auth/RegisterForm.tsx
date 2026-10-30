"use client";

import { useActionState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { Select } from "@/components/ui/Select";
import { AuthSubmitButton } from "./AuthSubmitButton";
import { type AuthState, registerAction } from "./actions";
import { loginPath } from "./returnUrlShared";
import { useCredentialForm } from "./useCredentialForm";
import { MIN_PASSWORD, MIN_USERNAME, validateRegister } from "./validation";

const initial: AuthState = { ok: false };

const ROLE_OPTIONS = [
  { value: "buyer", label: "Người mua (buyer)" },
  { value: "seller", label: "Người bán (seller)" },
];

export function RegisterForm({ returnUrl }: { returnUrl?: string }) {
  const [state, action] = useActionState(registerAction, initial);
  const form = useCredentialForm(state, validateRegister);

  return (
    <div className="mx-auto w-full max-w-sm">
      <Card>
        <CardContent>
          <div className="space-y-5">
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
              {returnUrl ? (
                <input type="hidden" name="returnUrl" value={returnUrl} />
              ) : null}
              <Input
                label="Tên đăng nhập"
                error={form.errors.username}
                helperText={
                  form.errors.username
                    ? undefined
                    : `Tối thiểu ${MIN_USERNAME} ký tự.`
                }
                ref={form.usernameRef}
                id="username"
                name="username"
                required
                autoComplete="username"
                placeholder="Chọn tên đăng nhập"
                onChange={() => form.onChange("username")}
                onBlur={() => form.onBlur("username")}
              />

              <Input
                label="Mật khẩu"
                error={form.errors.password}
                helperText={
                  form.errors.password
                    ? undefined
                    : `Tối thiểu ${MIN_PASSWORD} ký tự.`
                }
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

              <Select
                label="Loại tài khoản"
                id="role"
                name="role"
                defaultValue="buyer"
                options={ROLE_OPTIONS}
              />

              {state.error && !form.hasFieldErrors && (
                <Alert type="error" description={state.error} />
              )}

              <AuthSubmitButton>Đăng ký</AuthSubmitButton>

              <p className="text-center text-sm text-text-secondary">
                Đã có tài khoản?{" "}
                <span className="font-medium">
                  <Link href={loginPath(returnUrl)}>Đăng nhập ngay</Link>
                </span>
              </p>
            </form>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
