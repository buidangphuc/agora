"use server";

import { revalidatePath } from "next/cache";

import { answerQuestion, askQuestion } from "@/lib/gateway/engagement";

export async function askQuestionAction(
  listingId: string,
  questionText: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await askQuestion(listingId, questionText.trim());
    revalidatePath(`/listing/${listingId}`);
    return { ok: true, message: "Đã gửi câu hỏi tới shop!" };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Gửi câu hỏi thất bại.",
    };
  }
}

export async function answerQuestionAction(
  listingId: string,
  questionId: string,
  answerText: string,
  isShopReply = true,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await answerQuestion(questionId, answerText.trim(), isShopReply);
    revalidatePath(`/listing/${listingId}`);
    return { ok: true, message: "Đã gửi câu trả lời!" };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Gửi câu trả lời thất bại.",
    };
  }
}
