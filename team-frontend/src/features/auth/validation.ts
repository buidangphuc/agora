export interface CredentialValues {
  username: string;
  password: string;
}

export interface RegisterValues extends CredentialValues {
  acceptTerms?: boolean;
  acceptPrivacy?: boolean;
}

export type CredentialField =
  | "username"
  | "password"
  | "acceptTerms"
  | "acceptPrivacy";
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

/** Register enforces the minimum lengths (3 / 4) the server also checks. */
export function validateRegister({
  username,
  password,
  acceptTerms,
  acceptPrivacy,
}: RegisterValues): CredentialErrors {
  const errors: CredentialErrors = {};
  if (username.trim().length < MIN_USERNAME) {
    errors.username = `Tên đăng nhập tối thiểu ${MIN_USERNAME} ký tự.`;
  }
  if (password.length < MIN_PASSWORD) {
    errors.password = `Mật khẩu tối thiểu ${MIN_PASSWORD} ký tự.`;
  }
  if (!acceptTerms) {
    errors.acceptTerms = "Vui lòng đồng ý với Điều khoản sử dụng.";
  }
  if (!acceptPrivacy) {
    errors.acceptPrivacy = "Vui lòng đồng ý với Chính sách bảo mật.";
  }
  return errors;
}
