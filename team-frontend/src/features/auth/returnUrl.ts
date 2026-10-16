import "server-only";
import { redirect } from "next/navigation";
import { loginPath, safeReturnUrl } from "./returnUrlShared";

export { loginPath, safeReturnUrl };

export function redirectToLogin(returnTo?: string | null): never {
  redirect(loginPath(returnTo));
}
