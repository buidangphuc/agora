"use client";

import { useId, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { askQuestionAction } from "./actions";

/**
 * Ask-the-shop box (`qa-ask-form`). The submit is disabled while the text is
 * empty or the request is pending; success shows a toast and clears the field,
 * failure shows an error toast plus an inline Alert.
 */
export function QAAskForm({ listingId }: { listingId: string }) {
  const textId = useId();
  const toast = useToast();
  const [question, setQuestion] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function ask() {
    const text = question.trim();
    if (!text || pending) return;
    setError("");
    setPending(true);
    try {
      const res = await askQuestionAction(listingId, text);
      if (res.ok) {
        setQuestion("");
        toast.success("Đã gửi câu hỏi tới shop!");
      } else {
        setError(res.error);
        toast.error(res.error);
      }
    } catch {
      const message = "Gửi câu hỏi thất bại.";
      setError(message);
      toast.error(message);
    } finally {
      setPending(false);
    }
  }

  return (
    <div id="qa-ask" data-testid="qa-ask-form" className="space-y-3">
      <label htmlFor={textId} className="sr-only">
        Câu hỏi của bạn
      </label>
      <textarea
        id={textId}
        rows={2}
        value={question}
        disabled={pending}
        onChange={(e) => setQuestion(e.target.value)}
        placeholder="Đặt câu hỏi cho shop về sản phẩm này…"
        className="w-full rounded-lg border border-border-strong bg-surface-card p-2.5 text-sm text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:opacity-50"
      />
      {error && <Alert type="error" title={error} />}
      <Button
        variant="primary"
        size="md"
        isLoading={pending}
        disabled={!question.trim() || pending}
        onClick={ask}
      >
        Đặt câu hỏi cho Shop
      </Button>
    </div>
  );
}
