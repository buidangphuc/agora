"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import type { DeletionStatus } from "@/lib/gateway/privacy";
import { usePendingAction } from "../account/usePendingAction";
import { DeleteAccountModal } from "./DeleteAccountModal";
import { cancelDeletionAction } from "./actions";

function formatWhen(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.toLocaleDateString("vi-VN")} ${d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit" })}`;
}

export function DeletionCard({ status }: { status: DeletionStatus }) {
  const [pending, setPending] = useState(status.pending);
  const [eraseAfter, setEraseAfter] = useState(status.eraseAfter);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const { pending: busy, run } = usePendingAction();
  const toast = useToast();
  const router = useRouter();

  function cancel() {
    void run(async () => {
      setError("");
      const res = await cancelDeletionAction();
      if (res.ok) {
        setPending(false);
        toast.success("Đã hủy yêu cầu xóa tài khoản.");
        router.refresh();
      } else {
        setError(res.error);
      }
    });
  }

  if (pending) {
    return (
      <div className="flex flex-col gap-3">
        <p className="text-sm">
          Tài khoản của bạn sẽ bị xóa vào{" "}
          {eraseAfter ? formatWhen(eraseAfter) : "ngày đã hẹn"}. Bạn có thể hủy
          trước thời điểm này.
        </p>
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
        <div>
          <Button variant="outline" isLoading={busy} onClick={cancel}>
            Hủy yêu cầu xóa
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-text-secondary">
        Xóa tài khoản sẽ xóa dữ liệu cá nhân của bạn sau thời gian chờ.
      </p>
      <div>
        <Button variant="danger" onClick={() => setOpen(true)}>
          Xóa tài khoản
        </Button>
      </div>
      <DeleteAccountModal
        isOpen={open}
        onClose={() => setOpen(false)}
        onScheduled={(at) => {
          setOpen(false);
          setPending(true);
          setEraseAfter(at ?? "");
          router.refresh();
        }}
      />
    </div>
  );
}
