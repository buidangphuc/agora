export interface CredentialValues {
  username: string;
  password: string;
}

export type CredentialField = "username" | "password";
export type CredentialErrors = Partial<Record<CredentialField, string>>;

export const MIN_USERNAME = 3;
export const MIN_PASSWORD = 4;

/** Login only requires both fields to be filled. */
export function validateLogin({
  username,
  password,
}: CredentialValues): CredentialErrors {
  const errors: CredentialErrors = {};
  if (username.trim() === "") errors.username = "Vui lòng nhập tên đăng nhập.";
  if (password === "") errors.password = "Vui lòng nhập mật khẩu.";
  return errors;
}
