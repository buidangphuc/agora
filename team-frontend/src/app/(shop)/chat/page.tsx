import { redirect } from "next/navigation";

import { ChatView } from "@/features/chat/ChatView";
import {
  type ViewChatMessage,
  type ViewChatThread,
  getThreadMessages,
  listThreads,
} from "@/lib/gateway/chat";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function ChatPage({
  searchParams,
}: {
  searchParams?: { thread?: string };
}) {
  const me = getPrincipal();
  if (!me) redirect("/login?returnUrl=/chat");

  const threads = await listThreads();
  const targetId =
    searchParams?.thread || (threads.length > 0 ? threads[0].id : undefined);

  let activeThread: ViewChatThread | undefined;
  let initialMessages: ViewChatMessage[] = [];

  if (targetId) {
    activeThread = threads.find((t) => t.id === targetId);
    try {
      initialMessages = await getThreadMessages(targetId);
    } catch {
      // User unauthorized or thread not found: fallback gracefully
      activeThread = undefined;
      initialMessages = [];
    }
  }

  return (
    <section className="py-4">
      <ChatView
        threads={threads}
        activeThread={activeThread}
        initialMessages={initialMessages}
        currentUserId={me.id}
      />
    </section>
  );
}
