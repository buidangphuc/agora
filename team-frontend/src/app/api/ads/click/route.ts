import { recordAdClick } from "@/lib/gateway/promotion";

const TOKEN_RE = /^[A-Za-z0-9._~-]{1,256}$/;

export async function POST(req: Request): Promise<Response> {
  let token: unknown;
  try {
    token = ((await req.json()) as { token?: unknown } | null)?.token;
  } catch {
    return new Response(null, { status: 400 });
  }
  if (typeof token !== "string" || !TOKEN_RE.test(token)) {
    return new Response(null, { status: 400 });
  }
  try {
    await recordAdClick(token);
  } catch {
    console.warn("ads click: record failed");
  }
  return new Response(null, { status: 204 });
}

export function GET(_req: Request): Response {
  return new Response(null, { status: 405, headers: { Allow: "POST" } });
}
