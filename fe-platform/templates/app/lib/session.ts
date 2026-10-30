import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export const SESSION_COOKIE = "session";

export async function getToken(): Promise<string | undefined> {
  const jar = await cookies();
  return jar.get(SESSION_COOKIE)?.value || undefined;
}

export async function requireSession(): Promise<string> {
  const token = await getToken();
  if (!token) redirect("/");
  return token;
}
