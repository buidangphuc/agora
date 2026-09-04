import type { ViewReviewSummary } from "@/lib/gateway/ai";

/**
 * AI-generated summary of a listing's reviews (team-ai SummarizeReviews via the
 * gateway). Presentational — data is fetched server-side. Thin.
 */
export function AiReviewSummary({ summary }: { summary: ViewReviewSummary }) {
  return (
    <div className="rounded-2xl border border-indigo-100 bg-gradient-to-br from-indigo-50/80 to-blue-50/40 p-5 shadow-2xs">
      <h3 className="flex items-center gap-2 text-sm font-bold text-indigo-900">
        <span>✨</span>
        <span>Tóm tắt đánh giá bằng AI</span>
        {summary.sentiment && (
          <span className="rounded-full bg-white px-2 py-0.5 text-[10px] font-semibold text-indigo-700">
            {summary.sentiment}
          </span>
        )}
      </h3>

      {summary.summary && (
        <p className="mt-2 text-xs leading-relaxed text-gray-700">
          {summary.summary}
        </p>
      )}

      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
        {summary.pros.length > 0 && (
          <div>
            <p className="text-[11px] font-bold text-emerald-700">👍 Ưu điểm</p>
            <ul className="mt-1 space-y-0.5 text-xs text-gray-600">
              {summary.pros.map((p) => (
                <li key={p}>• {p}</li>
              ))}
            </ul>
          </div>
        )}
        {summary.cons.length > 0 && (
          <div>
            <p className="text-[11px] font-bold text-red-600">👎 Hạn chế</p>
            <ul className="mt-1 space-y-0.5 text-xs text-gray-600">
              {summary.cons.map((c) => (
                <li key={c}>• {c}</li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  );
}
