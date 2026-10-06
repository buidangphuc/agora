import { AiAssistantModal } from "@/features/ai/AiAssistantModal";
import { loadFeed } from "./data";

/** The floating assistant, fed by the same cached feed as FeedBlock. */
export async function AssistantBlock() {
  const { items } = await loadFeed();
  return <AiAssistantModal listings={items} />;
}
