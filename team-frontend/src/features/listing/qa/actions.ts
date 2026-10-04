"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import { answerQuestion, askQuestion } from "@/lib/gateway/engagement";

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message !== "" ? err.message : fallback;
}

export async function askQuestionAction(
  listingId: string,
  questionText: string,
): Promise<ActionResult> {
  try {
    await askQuestion(listingId, questionText.trim());
    revalidatePath(`/listing/${listingId}`);
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Gửi câu hỏi thất bại."));
  }
}

export async function answerQuestionAction(
  listingId: string,
  questionId: string,
  answerText: string,
  isShopReply = true,
): Promise<ActionResult> {
  try {
    await answerQuestion(questionId, answerText.trim(), isShopReply);
    revalidatePath(`/listing/${listingId}`);
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Gửi câu trả lời thất bại."));
  }
}
