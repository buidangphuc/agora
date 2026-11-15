"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { usePendingAction } from "../account/usePendingAction";
import { exportDataAction } from "./actions";

export function ExportButton() {
  const [error, setError] = useState("");
  const { pending, run } = usePendingAction();

  function download() {
    void run(async () => {
      setError("");
      const res = await exportDataAction();
      if (!res.ok || !res.data) {
        setError(res.ok ? "Chưa tải được dữ liệu. Vui lòng thử lại sau." : res.error);
        return;
      }
      const blob = new Blob([res.data.content], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = res.data.filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    });
  }

  return (
    <div className="flex flex-col gap-2">
      <div>
        <Button isLoading={pending} onClick={download}>
          Tải dữ liệu của tôi
        </Button>
      </div>
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
