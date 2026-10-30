"use client";

import { useActionState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { AuthSwitch } from "./AuthPanel";
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
    <div className="flex flex-col gap-6">
      <form
        action={action}
        onSubmit={form.onSubmit}
        noValidate
        className="flex flex-col gap-5"
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
      </form>
      <AuthSwitch prompt="Chưa có tài khoản?">
        <Link
          href={registerPath(returnUrl)}
          variant="secondary"
          size="lg"
          fullWidth
        >
          Đăng ký ngay
        </Link>
      </AuthSwitch>
    </div>
  );
}
