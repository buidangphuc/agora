"use client";

import { type FormEvent, useEffect, useRef, useState } from "react";

import type { AuthState } from "./actions";
import type {
  CredentialErrors,
  CredentialField,
  RegisterValues,
} from "./validation";

/**
 * Client-side state shared by LoginForm and RegisterForm: uncontrolled username
 * (restored from the server result after a failed attempt) and password
 * (cleared), validation on submit and on blur, field help from the server, and
 * focus on the first invalid control. Both inputs stay uncontrolled so React
 * hydration never overwrites what the user typed before the page was
 * interactive.
 */
export function useCredentialForm(
  state: AuthState,
  validate: (values: RegisterValues) => CredentialErrors,
) {
  const [errors, setErrors] = useState<CredentialErrors>({});
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const termsRef = useRef<HTMLInputElement>(null);
  const privacyRef = useRef<HTMLInputElement>(null);

  // A new server result: show any field help it names, and clear the password.
  useEffect(() => {
    if (state.ok || !state.error) return;
    setErrors(state.fields ?? {});
    if (passwordRef.current) passwordRef.current.value = "";
    if (usernameRef.current && state.username !== undefined) {
      usernameRef.current.value = state.username;
    }
  }, [state]);

  function values(): RegisterValues {
    return {
      username: usernameRef.current?.value ?? "",
      password: passwordRef.current?.value ?? "",
      acceptTerms: termsRef.current?.checked ?? false,
      acceptPrivacy: privacyRef.current?.checked ?? false,
    };
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    const found = validate(values());
    setErrors(found);
    if (Object.keys(found).length === 0) return;
    // Stops the form action: no request is sent.
    event.preventDefault();
    (found.username
      ? usernameRef
      : found.password
        ? passwordRef
        : found.acceptTerms
          ? termsRef
          : privacyRef
    ).current?.focus();
  }

  function onBlur(field: CredentialField) {
    const found = validate(values());
    setErrors((prev) => ({ ...prev, [field]: found[field] }));
  }

  function onChange(field: CredentialField) {
    setErrors((prev) => (prev[field] ? { ...prev, [field]: undefined } : prev));
  }

  return {
    errors,
    usernameRef,
    passwordRef,
    termsRef,
    privacyRef,
    onSubmit,
    onBlur,
    onChange,
    hasFieldErrors: Object.values(errors).some(Boolean),
  };
}
