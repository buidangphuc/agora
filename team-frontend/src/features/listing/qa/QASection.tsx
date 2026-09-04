"use client";

import Link from "next/link";
import { useState, useTransition } from "react";

import type { ViewQuestion } from "@/lib/gateway/engagement";
import { answerQuestionAction, askQuestionAction } from "./actions";

function AnswerForm({
  listingId,
  questionId,
}: {
  listingId: string;
  questionId: string;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [pending, start] = useTransition();

  if (!open) {
    return (
      <button
        type="button"
        data-testid="qa-answer-toggle"
        onClick={() => setOpen(true)}
        className="mt-1 text-[11px] font-medium text-brand hover:underline"
      >
        + Trả lời (Shop)
      </button>
    );
  }

  function submit() {
    if (!text.trim()) {
      setError("Vui lòng nhập nội dung trả lời.");
      return;
    }
    setError("");
    start(async () => {
      const res = await answerQuestionAction(listingId, questionId, text.trim());
      if (res.ok) {
        setText("");
        setOpen(false);
      } else {
        setError(res.message || "Gửi câu trả lời thất bại.");
      }
    });
  }

  return (
    <div className="mt-2 space-y-1.5">
      <textarea
        aria-label="Nội dung trả lời"
        rows={2}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Nhập câu trả lời của shop…"
        className="w-full rounded-lg border border-gray-300 p-2 text-xs text-gray-900 focus:border-brand focus:outline-hidden"
      />
      {error && <p className="text-[11px] text-red-600">{error}</p>}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={submit}
          disabled={pending}
          className="rounded-lg bg-brand px-3 py-1.5 text-[11px] font-semibold text-white hover:bg-brand-dark disabled:opacity-60"
        >
          {pending ? "Đang gửi..." : "Gửi trả lời"}
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="rounded-lg border px-3 py-1.5 text-[11px] font-medium text-gray-600 hover:bg-gray-50"
        >
          Hủy
        </button>
      </div>
    </div>
  );
}

function QuestionItem({ question }: { question: ViewQuestion }) {
  return (
    <div
      data-testid="qa-item"
      className="space-y-1.5 rounded-xs border border-slate-100 bg-slate-50/50 p-3"
    >
      <div className="flex items-start gap-1.5 text-xs font-bold text-gray-800">
        <span className="font-black text-brand">H:</span>
        <span>{question.questionText}</span>
      </div>

      {question.answers.map((a) => (
        <div
          key={a.id}
          data-testid="qa-answer"
          className="flex items-start gap-1.5 pl-3 text-xs text-gray-600"
        >
          <span className="font-bold text-emerald-600">Đ:</span>
          <span>
            {a.answerText}
            {a.isShopReply && (
              <span className="ml-1.5 inline-flex items-center rounded-2xs bg-emerald-50 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700">
                Shop
              </span>
            )}
          </span>
        </div>
      ))}

      <AnswerForm listingId={question.listingId} questionId={question.id} />
    </div>
  );
}

export function QASection({
  listingId,
  loggedIn,
  initialQuestions,
}: {
  listingId: string;
  loggedIn: boolean;
  initialQuestions: ViewQuestion[];
}) {
  const [question, setQuestion] = useState("");
  const [error, setError] = useState("");
  const [ok, setOk] = useState("");
  const [pending, start] = useTransition();

  function ask() {
    if (!question.trim()) {
      setError("Vui lòng nhập câu hỏi.");
      return;
    }
    setError("");
    setOk("");
    start(async () => {
      const res = await askQuestionAction(listingId, question.trim());
      if (res.ok) {
        setQuestion("");
        setOk(res.message || "Đã gửi câu hỏi tới shop!");
      } else {
        setError(res.message || "Gửi câu hỏi thất bại.");
      }
    });
  }

  return (
    <div className="space-y-4 rounded-xs bg-white p-6 shadow-2xs">
      <h2 className="text-xs font-bold uppercase tracking-wider text-gray-800">
        HỎI ĐÁP VỀ SẢN PHẨM (Q&amp;A)
      </h2>

      {/* Ask box — gated on session, mirroring the listing's other actions. */}
      {loggedIn ? (
        <div data-testid="qa-ask-form" className="space-y-1.5">
          <textarea
            aria-label="Câu hỏi của bạn"
            rows={2}
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            placeholder="Đặt câu hỏi cho shop về sản phẩm này…"
            className="w-full rounded-lg border border-gray-300 p-2.5 text-xs text-gray-900 focus:border-brand focus:outline-hidden"
          />
          {error && <p className="text-[11px] text-red-600">{error}</p>}
          {ok && <p className="text-[11px] text-emerald-600">{ok}</p>}
          <button
            type="button"
            onClick={ask}
            disabled={pending}
            className="rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark disabled:opacity-60"
          >
            {pending ? "Đang gửi..." : "Đặt câu hỏi cho Shop"}
          </button>
        </div>
      ) : (
        <p data-testid="qa-login-prompt" className="text-xs text-gray-500">
          <Link
            href={`/login?returnUrl=/listing/${listingId}`}
            className="font-semibold text-brand hover:underline"
          >
            Đăng nhập
          </Link>{" "}
          để đặt câu hỏi cho shop.
        </p>
      )}

      {/* Questions + answers list */}
      <div className="space-y-3">
        {initialQuestions.length === 0 ? (
          <div
            data-testid="qa-empty"
            className="py-6 text-center text-xs text-gray-400"
          >
            Chưa có câu hỏi nào. Hãy là người đầu tiên hỏi shop!
          </div>
        ) : (
          initialQuestions.map((q) => <QuestionItem key={q.id} question={q} />)
        )}
      </div>
    </div>
  );
}
