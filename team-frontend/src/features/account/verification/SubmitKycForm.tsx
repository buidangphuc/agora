"use client";

import { type FormEvent, useState } from "react";

import { Button } from "@/components/ui/Button";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { submitKycAction } from "./actions";

const DOC_TYPES = [
  { value: "national_id", label: "CMND/CCCD" },
  { value: "passport", label: "Hộ chiếu" },
  { value: "business_license", label: "Giấy phép kinh doanh" },
];

/**
 * Submit a KYC document for review. Every control is labelled through
 * FormItem (label htmlFor = control id, help as aria-describedby). Submit stays
 * disabled until the document reference is non-empty.
 */
export function SubmitKycForm() {
  const [docType, setDocType] = useState(DOC_TYPES[0].value);
  const [docRef, setDocRef] = useState("");
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void run(async () => {
      const res = await submitKycAction(docType, docRef);
      if (res.ok) {
        setDocRef("");
        toast.success("Đã gửi hồ sơ xác minh.");
      } else {
        toast.error(res.error);
      }
    });
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <FormItem label="Loại giấy tờ">
        <Select
          className="min-h-10"
          name="docType"
          value={docType}
          onChange={(e) => setDocType(e.target.value)}
          options={DOC_TYPES}
        />
      </FormItem>
      <FormItem
        label="Mã tham chiếu tài liệu"
        help="Nhập số giấy tờ hoặc khóa tệp đã tải lên."
      >
        <Input
          className="min-h-10"
          name="docRef"
          required
          value={docRef}
          onChange={(e) => setDocRef(e.target.value)}
          placeholder="Ví dụ: 0123456789"
        />
      </FormItem>
      <Button
        type="submit"
        isLoading={pending}
        disabled={!docRef.trim()}
        className="min-h-10 w-full sm:w-auto"
      >
        Gửi hồ sơ xác minh
      </Button>
    </form>
  );
}
