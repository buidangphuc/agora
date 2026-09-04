"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { submitKycAction } from "./actions";

const DOC_TYPES = [
  { value: "national_id", label: "CMND/CCCD" },
  { value: "passport", label: "Hộ chiếu" },
  { value: "business_license", label: "Giấy phép kinh doanh" },
];

/**
 * Submit a KYC document for review. Wired to team-verification
 * VerificationService through the gateway via a server action. Thin.
 */
export function SubmitKycForm() {
  const [docType, setDocType] = useState(DOC_TYPES[0].value);
  const [docRef, setDocRef] = useState("");
  const [pending, start] = useTransition();
  const toast = useToast();

  function submit() {
    start(async () => {
      const res = await submitKycAction(docType, docRef);
      if (res.ok) {
        setDocRef("");
        toast.success("✓ Đã gửi hồ sơ xác minh.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="space-y-3">
      <div>
        <label className="mb-1 block text-xs font-medium text-gray-600">
          Loại giấy tờ
        </label>
        <select
          value={docType}
          onChange={(e) => setDocType(e.target.value)}
          className="w-full rounded-md border border-gray-200 px-3 py-1.5 text-sm focus:border-brand focus:outline-none"
        >
          {DOC_TYPES.map((d) => (
            <option key={d.value} value={d.value}>
              {d.label}
            </option>
          ))}
        </select>
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-gray-600">
          Mã tham chiếu tài liệu
        </label>
        <input
          type="text"
          value={docRef}
          onChange={(e) => setDocRef(e.target.value)}
          placeholder="Ví dụ: số giấy tờ hoặc khóa tệp đã tải lên"
          className="w-full rounded-md border border-gray-200 px-3 py-1.5 text-sm focus:border-brand focus:outline-none"
        />
      </div>
      <button
        type="button"
        onClick={submit}
        disabled={pending || !docRef.trim()}
        className="rounded-md bg-brand px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        {pending ? "Đang gửi..." : "Gửi hồ sơ xác minh"}
      </button>
    </div>
  );
}
