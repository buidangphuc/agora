"use client";

import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";

import { createKycUploadUrlAction, submitKycAction } from "./actions";
import { validateKycFiles } from "./status";

const UPLOAD_FAILED = "Tải tệp thất bại, vui lòng thử lại";

export function KycUploadForm() {
  const router = useRouter();
  const [files, setFiles] = useState<File[]>([]);
  const [consent, setConsent] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const refs = useRef(new Map<File, { uploadUrl: string; docRef: string }>());

  const validation = validateKycFiles(files);
  const canSubmit = validation.ok && consent && !pending;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit || busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const docRefs: string[] = [];
      for (const file of files) {
        let issued = refs.current.get(file);
        if (!issued) {
          const res = await createKycUploadUrlAction(file.type, file.size);
          if (!res.ok || !res.data) {
            setError(res.ok ? UPLOAD_FAILED : res.error);
            return;
          }
          issued = res.data;
          refs.current.set(file, issued);
        }
        let put: Response;
        try {
          put = await fetch(issued.uploadUrl, {
            method: "PUT",
            headers: { "Content-Type": file.type },
            body: file,
          });
        } catch {
          refs.current.delete(file);
          setError(UPLOAD_FAILED);
          return;
        }
        if (!put.ok) {
          refs.current.delete(file);
          setError(UPLOAD_FAILED);
          return;
        }
        const ref = issued.docRef;
        docRefs.push(ref);
      }
      const res = await submitKycAction("national_id", docRefs);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      router.refresh();
    } finally {
      busy.current = false;
      setPending(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate className="max-w-xl space-y-4">
      <div className="space-y-2">
        <input
          type="file"
          multiple
          accept="image/jpeg,image/png,application/pdf"
          disabled={pending}
          aria-label="Tệp giấy tờ"
          onChange={(e) => {
            refs.current = new Map();
            setError("");
            setFiles(Array.from(e.target.files ?? []));
          }}
        />
        {validation.limitError && (
          <p role="alert" className="text-sm text-red-600">
            {validation.limitError}
          </p>
        )}
        <ul className="space-y-1">
          {files.map((file, i) => (
            <li key={`${file.name}-${i}`} className="text-sm">
              {file.name}
              {validation.fileErrors[i] && (
                <span className="ml-2 text-red-600">
                  {validation.fileErrors[i]}
                </span>
              )}
            </li>
          ))}
        </ul>
      </div>
      <Checkbox
        label="Tôi đồng ý cho phép Agora xử lý giấy tờ để xác minh danh tính"
        checked={consent}
        onCheckedChange={(v) => setConsent(v === true)}
      />
      {error && <Alert type="error" description={error} />}
      <Button type="submit" disabled={!canSubmit} isLoading={pending}>
        Gửi hồ sơ xác minh
      </Button>
    </form>
  );
}
