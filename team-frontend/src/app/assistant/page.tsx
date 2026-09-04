import { AssistantChatView } from "@/features/assistant/AssistantChatView";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Trợ Lý Mua Sắm AI Thông Minh | Sàn Thương Mại Điện Tử",
  description:
    "Trợ lý ảo RAG AI hiểu ngôn ngữ tự nhiên, gợi ý sản phẩm và so sánh giá tức thì.",
};

export default function AssistantPage() {
  return <AssistantChatView />;
}
