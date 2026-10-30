"use client";

import { useActionState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { Select } from "@/components/ui/Select";
import { AuthSwitch } from "./AuthPanel";
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
      </form>
      <AuthSwitch prompt="Đã có tài khoản?">
        <Link
          href={loginPath(returnUrl)}
          variant="secondary"
          size="lg"
          fullWidth
        >
          Đăng nhập ngay
        </Link>
      </AuthSwitch>
    </div>
  );
}
