"use client";

import { type RefObject, useActionState } from "react";

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

function AcceptBox({
  name,
  inputRef,
  error,
  onChange,
  href,
  label,
}: {
  name: string;
  inputRef: RefObject<HTMLInputElement | null>;
  error?: string;
  onChange: () => void;
  href: string;
  label: string;
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          name={name}
          ref={inputRef}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={error ? `${name}-error` : undefined}
          onChange={onChange}
          className="mt-1"
        />
        <span>
          Tôi đã đọc và chấp nhận{" "}
          <a href={href} target="_blank" rel="noreferrer" className="underline">
            {label}
          </a>
        </span>
      </label>
      {error ? (
        <p id={`${name}-error`} className="text-xs text-text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function RegisterForm({
  returnUrl,
  referralRef,
}: {
  returnUrl?: string;
  referralRef?: string;
}) {
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
        {referralRef ? (
          <input type="hidden" name="ref" value={referralRef} />
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

        <AcceptBox
          name="acceptTerms"
          inputRef={form.termsRef}
          error={form.errors.acceptTerms}
          onChange={() => form.onChange("acceptTerms")}
          href="/dieu-khoan-su-dung"
          label="Điều khoản sử dụng"
        />
        <AcceptBox
          name="acceptPrivacy"
          inputRef={form.privacyRef}
          error={form.errors.acceptPrivacy}
          onChange={() => form.onChange("acceptPrivacy")}
          href="/chinh-sach-bao-mat"
          label="Chính sách bảo mật"
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
