import Link from "next/link";

import { Avatar } from "@/components/ui/Avatar";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import {
  type ViewQuestion,
  listQuestionsByListing,
} from "@/lib/gateway/engagement";
import { QAAnswerForm } from "./QAAnswerForm";

const linkButton =
  "inline-flex items-center justify-center rounded-lg border border-border-strong bg-surface-card px-4 py-2 text-sm font-medium text-text-primary shadow-sm transition duration-150 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2";

function QuestionItem({ question }: { question: ViewQuestion }) {
  return (
    <li
      data-testid="qa-item"
      className="space-y-3 rounded-xl border border-border-subtle bg-surface-muted p-4"
    >
      <div className="flex items-start gap-3">
        <Avatar name="Người mua" size="sm" />
        <div className="min-w-0 flex-1 text-sm font-semibold text-text-primary">
          {question.questionText}
        </div>
      </div>

      {question.answers.map((a) => (
        <div
          key={a.id}
          data-testid="qa-answer"
          className="flex items-start gap-3 pl-6 text-sm text-text-secondary"
        >
          <Avatar name={a.isShopReply ? "Shop" : "Người mua"} size="sm" />
          <div className="min-w-0 flex-1 space-y-1">
            {a.isShopReply && <Tag color="success">Shop</Tag>}
            <p>{a.answerText}</p>
          </div>
        </div>
      ))}

      <QAAnswerForm listingId={question.listingId} questionId={question.id} />
    </li>
  );
}

/** The questions of one listing, streamed in their own Suspense boundary. */
export async function QuestionList({
  listingId,
  loggedIn,
}: {
  listingId: string;
  loggedIn: boolean;
}) {
  const questions = await listQuestionsByListing(listingId);

  if (questions.length === 0) {
    return (
      <div data-testid="qa-empty">
        <Empty
          description="Chưa có câu hỏi nào. Hãy là người đầu tiên hỏi shop!"
          action={
            loggedIn ? (
              <a href="#qa-ask" className={linkButton}>
                Đặt câu hỏi
              </a>
            ) : (
              <Link
                href={`/login?returnUrl=/listing/${listingId}`}
                className={linkButton}
              >
                Đăng nhập để hỏi
              </Link>
            )
          }
        />
      </div>
    );
  }

  return (
    <ul className="space-y-3">
      {questions.map((q) => (
        <QuestionItem key={q.id} question={q} />
      ))}
    </ul>
  );
}
