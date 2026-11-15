"use client";

import { useEffect, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { useToast } from "@/components/ui/ToastProvider";
import { readConsent, writeConsent } from "@/lib/consent/consent";
import type { ConsentEntry } from "@/lib/gateway/privacy";
import { useRouter } from "next/navigation";
import { usePendingAction } from "../account/usePendingAction";
import { saveConsentAction } from "./actions";

const CHANGE_EVENT = "agora:consent:change";

type Purpose = "ANALYTICS" | "MARKETING" | "PERSONALIZATION";
type Values = Record<Purpose, boolean>;

const ROWS: { purpose: Purpose; label: string; hint: string }[] = [
  {
    purpose: "ANALYTICS",
    label: "Phân tích",
    hint: "Giúp chúng tôi hiểu cách bạn dùng Agora để cải thiện dịch vụ.",
  },
  {
    purpose: "MARKETING",
    label: "Tiếp thị",
    hint: "Cho phép đo lường và hiển thị quảng cáo phù hợp.",
  },
  {
    purpose: "PERSONALIZATION",
    label: "Cá nhân hóa",
    hint: "Gợi ý sản phẩm dựa trên hoạt động của bạn.",
  },
];

function fromRecords(records: ConsentEntry[]): Partial<Values> {
  const out: Partial<Values> = {};
  for (const r of records) {
    if (
      r.purpose !== "ANALYTICS" &&
      r.purpose !== "MARKETING" &&
      r.purpose !== "PERSONALIZATION"
    )
      continue;
    if (!(r.purpose in out)) out[r.purpose] = r.granted;
  }
  return out;
}

export function ConsentForm({ records }: { records: ConsentEntry[] }) {
  const recorded = fromRecords(records);
  const [saved, setSaved] = useState<Values>({
    ANALYTICS: recorded.ANALYTICS ?? false,
    MARKETING: recorded.MARKETING ?? false,
    PERSONALIZATION: recorded.PERSONALIZATION ?? false,
  });
  const [values, setValues] = useState<Values>(saved);
  const [error, setError] = useState("");
  const { pending, run } = usePendingAction();
  const toast = useToast();
  const router = useRouter();

  useEffect(() => {
    const c = readConsent();
    const recorded = fromRecords(records);
    const base: Values = {
      ANALYTICS: recorded.ANALYTICS ?? c.analytics,
      MARKETING: recorded.MARKETING ?? c.marketing,
      PERSONALIZATION: recorded.PERSONALIZATION ?? c.personalization,
    };
    setSaved(base);
    setValues(base);
  }, [records]);

  const changes = ROWS.filter(
    (r) => values[r.purpose] !== saved[r.purpose],
  ).map((r) => ({ purpose: r.purpose, granted: values[r.purpose] }));

  function save() {
    if (changes.length === 0) return;
    void run(async () => {
      setError("");
      const res = await saveConsentAction(changes);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      writeConsent({
        analytics: values.ANALYTICS,
        marketing: values.MARKETING,
        personalization: values.PERSONALIZATION,
      });
      window.dispatchEvent(new Event(CHANGE_EVENT));
      setSaved(values);
      toast.success("Đã lưu lựa chọn của bạn.");
      router.refresh();
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {ROWS.map((r) => (
        <div key={r.purpose} className="flex flex-col gap-1">
          <Checkbox
            label={r.label}
            checked={values[r.purpose]}
            onCheckedChange={(v) =>
              setValues((prev) => ({ ...prev, [r.purpose]: v }))
            }
          />
          <p className="text-sm text-text-secondary">{r.hint}</p>
        </div>
      ))}
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
      <div>
        <Button
          isLoading={pending}
          disabled={changes.length === 0}
          onClick={save}
        >
          Lưu lựa chọn
        </Button>
      </div>
    </div>
  );
}
