"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { answerQuestionAction } from "./actions";

/**
 * Shop reply to one question: a toggle (`qa-answer-toggle`) that opens a textarea.
 * Submit is disabled when empty or pending; success toasts and closes, failure
 * toasts and shows an inline Alert.
 */
export function QAAnswerForm({
  listingId,
  questionId,
}: {
  listingId: string;
  questionId: string;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  if (!open) {
    return (
      <Button
        data-testid="qa-answer-toggle"
        variant="ghost"
        size="sm"
        onClick={() => setOpen(true)}
        className="underline"
      >
        + Trả lời (Shop)
      </Button>
    );
  }

  async function submit() {
    const answer = text.trim();
    if (!answer || pending) return;
    setError("");
    setPending(true);
    try {
      const res = await answerQuestionAction(listingId, questionId, answer);
      if (res.ok) {
        setText("");
        setOpen(false);
        toast.success("Đã gửi câu trả lời!");
      } else {
        setError(res.error);
        toast.error(res.error);
      }
    } catch {
      const message = "Gửi câu trả lời thất bại.";
      setError(message);
      toast.error(message);
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="space-y-2">
      <textarea
        aria-label="Nội dung trả lời"
        rows={2}
        value={text}
        disabled={pending}
        onChange={(e) => setText(e.target.value)}
        placeholder="Nhập câu trả lời của shop…"
        className="w-full rounded-lg border border-border-strong bg-surface-card p-2 text-sm text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:opacity-50"
      />
      {error && <Alert type="error" title={error} />}
      <div className="flex gap-2">
        <Button
          variant="primary"
          size="sm"
          isLoading={pending}
          disabled={!text.trim() || pending}
          onClick={submit}
        >
          Gửi trả lời
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={pending}
          onClick={() => setOpen(false)}
        >
          Hủy
        </Button>
      </div>
    </div>
  );
}
