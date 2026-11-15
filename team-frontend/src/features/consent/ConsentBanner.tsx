"use client";

import { useEffect, useState } from "react";

import {
  type ConsentGroups,
  readConsent,
  writeConsent,
} from "@/lib/consent/consent";
import { recordBannerConsent } from "./actions";

const OPEN_EVENT = "agora:consent:open";
const CHANGE_EVENT = "agora:consent:change";

const GROUPS: { key: keyof ConsentGroups; label: string; hint: string }[] = [
  {
    key: "analytics",
    label: "Phân tích",
    hint: "Giúp chúng tôi hiểu cách trang web được sử dụng.",
  },
  {
    key: "marketing",
    label: "Tiếp thị",
    hint: "Giúp chúng tôi đo lường và cải thiện quảng cáo.",
  },
  {
    key: "personalization",
    label: "Cá nhân hóa",
    hint: "Điều chỉnh gợi ý theo hoạt động của bạn.",
  },
];

const NONE: ConsentGroups = {
  analytics: false,
  marketing: false,
  personalization: false,
};

export function ConsentBanner() {
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState<ConsentGroups>(NONE);

  useEffect(() => {
    const current = readConsent();
    if (current.version === 0) setOpen(true);
    const reopen = () => {
      const c = readConsent();
      setChoice(
        c.version === 0
          ? NONE
          : {
              analytics: c.analytics,
              marketing: c.marketing,
              personalization: c.personalization,
            },
      );
      setOpen(true);
    };
    window.addEventListener(OPEN_EVENT, reopen);
    return () => window.removeEventListener(OPEN_EVENT, reopen);
  }, []);

  if (!open) return null;

  const commit = (groups: ConsentGroups) => {
    const prev = readConsent();
    writeConsent(groups);
    recordBannerConsent(prev, groups).catch(() => {});
    window.dispatchEvent(new CustomEvent(CHANGE_EVENT, { detail: groups }));
    setOpen(false);
  };

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 p-4">
      <dialog
        open
        aria-labelledby="consent-title"
        className="static mx-auto w-full max-w-2xl rounded-lg border bg-surface-card p-4 shadow-xl text-sm text-text-secondary"
      >
        <h2 id="consent-title" className="font-bold text-text-primary mb-2">
          Tùy chọn cookie
        </h2>
        <div className="space-y-2 mb-4">
          {GROUPS.map((g) => (
            <label key={g.key} className="flex items-start gap-2">
              <input
                type="checkbox"
                checked={choice[g.key]}
                onChange={(e) =>
                  setChoice((prev) => ({ ...prev, [g.key]: e.target.checked }))
                }
                aria-label={g.label}
                className="mt-1"
              />
              <span>
                <span className="font-medium">{g.label}</span>
                <span className="block text-xs text-text-secondary">
                  {g.hint}
                </span>
              </span>
            </label>
          ))}
        </div>
        <div className="flex flex-wrap gap-2 justify-end">
          <button
            type="button"
            className="rounded border px-3 py-1.5"
            onClick={() => commit(NONE)}
          >
            Từ chối tất cả
          </button>
          <button
            type="button"
            className="rounded border px-3 py-1.5"
            onClick={() => commit(choice)}
          >
            Lưu lựa chọn
          </button>
          <button
            type="button"
            className="rounded bg-action-primary px-3 py-1.5 text-text-inverse"
            onClick={() =>
              commit({
                analytics: true,
                marketing: true,
                personalization: true,
              })
            }
          >
            Chấp nhận tất cả
          </button>
        </div>
      </dialog>
    </div>
  );
}

export function CookieSettingsLink() {
  return (
    <button
      type="button"
      className="underline hover:text-text-primary"
      onClick={() => window.dispatchEvent(new CustomEvent(OPEN_EVENT))}
    >
      Cài đặt cookie
    </button>
  );
}
