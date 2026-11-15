"use client";

import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";

import { logoutAction } from "@/features/auth/actions";

import { acceptCurrentLegal, getLegalAcceptance } from "./actions";

const KEY = "agora_legal_ok";
const LEGAL_PATHS = [
  "/dieu-khoan-su-dung",
  "/chinh-sach-bao-mat",
  "/quy-che-hoat-dong",
  "/chinh-sach-doi-tra",
  "/hang-cam-kinh-doanh",
  "/giai-quyet-khieu-nai",
  "/dieu-khoan-nguoi-ban",
];

function currentValue() {
  return `${process.env.NEXT_PUBLIC_LEGAL_TERMS_VERSION ?? "2026-11-13"}|${
    process.env.NEXT_PUBLIC_PRIVACY_POLICY_VERSION ?? "2026-11-13"
  }`;
}

export function LegalReacceptance() {
  const pathname = usePathname();
  const [needs, setNeeds] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (sessionStorage.getItem(KEY) === currentValue()) return;
    let live = true;
    getLegalAcceptance()
      .then((r) => {
        if (!live || !r.signedIn) return;
        if (r.needsAcceptance) setNeeds(true);
        else sessionStorage.setItem(KEY, currentValue());
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  if (!needs || LEGAL_PATHS.includes(pathname ?? "")) return null;

  const accept = async () => {
    setBusy(true);
    try {
      const res = await acceptCurrentLegal();
      if (res.ok) {
        sessionStorage.setItem(KEY, currentValue());
        setNeeds(false);
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-surface-overlay p-4">
      <dialog
        open
        aria-labelledby="legal-reaccept-title"
        className="static w-full max-w-md rounded-lg border bg-surface-card p-4 text-sm text-text-secondary shadow-xl"
      >
        <h2
          id="legal-reaccept-title"
          className="mb-2 font-bold text-text-primary"
        >
          Điều khoản đã được cập nhật
        </h2>
        <p className="mb-4">
          Vui lòng đọc{" "}
          <a href="/dieu-khoan-su-dung" className="underline">
            Điều khoản sử dụng
          </a>{" "}
          và{" "}
          <a href="/chinh-sach-bao-mat" className="underline">
            Chính sách bảo mật
          </a>{" "}
          và đồng ý để tiếp tục sử dụng tài khoản.
        </p>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            className="rounded border px-3 py-1.5"
            onClick={() => void logoutAction()}
          >
            Đăng xuất
          </button>
          <button
            type="button"
            disabled={busy}
            className="rounded bg-action-primary px-3 py-1.5 text-text-inverse"
            onClick={() => void accept()}
          >
            Đồng ý
          </button>
        </div>
      </dialog>
    </div>
  );
}
