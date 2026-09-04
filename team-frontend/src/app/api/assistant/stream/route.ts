import { makeClients } from "@/lib/gateway/client";
import { getToken } from "@/lib/gateway/session";

// Streams AI reply tokens to the browser. Runs server-side: the token comes from
// the session cookie, we open the gateway's ChatService.StreamChat (Connect
// server-streaming → team-ai), and relay each delta as a plain-text chunk. The
// browser reads it with a ReadableStream reader (typing effect). Rule 1 intact:
// the browser talks only to Next.js, which talks only to the gateway.
export const dynamic = "force-dynamic";

export async function POST(request: Request): Promise<Response> {
  const { message, sessionId } = (await request.json().catch(() => ({}))) as {
    message?: string;
    sessionId?: string;
  };
  if (!message || !message.trim()) {
    return new Response("missing message", { status: 400 });
  }

  const clients = makeClients(getToken());
  const encoder = new TextEncoder();

  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      try {
        for await (const res of clients.chat.streamChat({
          message,
          sessionId: sessionId ?? "",
        })) {
          if (res.delta) controller.enqueue(encoder.encode(res.delta));
          if (res.done) break;
        }
      } catch {
        // End the stream on upstream error; the client keeps whatever arrived.
      } finally {
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "no-cache, no-transform",
    },
  });
}
