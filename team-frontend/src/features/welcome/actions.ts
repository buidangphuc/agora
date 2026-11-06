"use server";

import { revalidatePath } from "next/cache";
import { cookies, headers } from "next/headers";

import { isSecureRequest } from "@/lib/gateway/session-cookie";

const COOKIE_BY_PART = {
  banner: "agora_welcome_dismissed",
  checklist: "agora_checklist_dismissed",
} as const;

export type WelcomePart = keyof typeof COOKIE_BY_PART;

const MAX_AGE_S = 14 * 24 * 60 * 60;

export async function dismissWelcome(part: WelcomePart): Promise<void> {
  const name = COOKIE_BY_PART[part];
  if (!name) return;
  (await cookies()).set(name, "1", {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: MAX_AGE_S,
    secure: isSecureRequest(await headers()),
  });
  revalidatePath("/", "layout");
}
