import React from "react";

import type { ViewDispute } from "@/lib/gateway/disputes";

export function OrderDisputeList({
  disputes,
  failed = false,
}: {
  disputes: ViewDispute[];
  failed?: boolean;
}) {
  return (
    <section aria-label="Khiếu nại" className="space-y-3">
      <h3 className="text-sm font-bold text-text-primary">Khiếu nại</h3>
      {failed ? (
        <p className="text-sm text-text-secondary">
          Không tải được danh sách khiếu nại.
        </p>
      ) : disputes.length === 0 ? (
        <p className="text-sm text-text-secondary">Chưa có khiếu nại</p>
      ) : (
        <ul className="space-y-3">
          {disputes.map((d) => (
            <li
              key={d.id}
              data-testid="dispute-item"
              className="space-y-1 rounded-lg border border-border-subtle bg-surface-card p-3 text-sm"
            >
              <p className="font-medium text-text-primary">{d.statusText}</p>
              {d.rulingText !== "" && (
                <p className="text-text-primary">{d.rulingText}</p>
              )}
              {d.resolution !== "" && (
                <p className="text-text-secondary">{d.resolution}</p>
              )}
              <p className="text-text-secondary">{d.reason}</p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
