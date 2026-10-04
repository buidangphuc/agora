"use client";

import { type FormEvent, useEffect, useRef, useState } from "react";

import type { AuthState } from "./actions";
import type {
  CredentialErrors,
  CredentialField,
  CredentialValues,
} from "./validation";

/**
 * Client-side state shared by LoginForm and RegisterForm: controlled username
 * (kept after a failed attempt), uncontrolled password (cleared), validation on
 * submit and on blur, field help from the server, and focus on the first
 * invalid control.
 */
export function useCredentialForm(
  state: AuthState,
  validate: (values: CredentialValues) => CredentialErrors,
) {
  const [username, setUsername] = useState("");
  const [errors, setErrors] = useState<CredentialErrors>({});
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  // A new server result: show any field help it names, and clear the password.
  useEffect(() => {
    if (state.ok || !state.error) return;
    setErrors(state.fields ?? {});
    if (passwordRef.current) passwordRef.current.value = "";
  }, [state]);

  function values(): CredentialValues {
    return {
      username: usernameRef.current?.value ?? "",
      password: passwordRef.current?.value ?? "",
    };
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    const found = validate(values());
    setErrors(found);
    if (Object.keys(found).length === 0) return;
    // Stops the form action: no request is sent.
    event.preventDefault();
    (found.username ? usernameRef : passwordRef).current?.focus();
  }

  function onBlur(field: CredentialField) {
    const found = validate(values());
    setErrors((prev) => ({ ...prev, [field]: found[field] }));
  }

  function onChange(field: CredentialField) {
    setErrors((prev) => (prev[field] ? { ...prev, [field]: undefined } : prev));
  }

  return {
    username,
    setUsername,
    errors,
    usernameRef,
    passwordRef,
    onSubmit,
    onBlur,
    onChange,
    hasFieldErrors: Object.values(errors).some(Boolean),
  };
}
