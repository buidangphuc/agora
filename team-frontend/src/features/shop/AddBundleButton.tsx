"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { loginPath } from "@/features/auth/returnUrlShared";
import { addBundleToCartAction } from "@/features/cart/actions";

export function AddBundleButton({
  bundleId,
  shopId,
}: {
  bundleId: string;
  shopId: string;
}) {
  const router = useRouter();
  const inFlight = useRef(false);
  const [pending, setPending] = useState(false);
  const [added, setAdded] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onClick() {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    try {
      const res = await addBundleToCartAction(bundleId);
      if (res.ok) {
        setAdded(true);
      } else if ("unauthenticated" in res && res.unauthenticated) {
        router.push(loginPath(`/shop/${shopId}`));
      } else if ("unavailable" in res && res.unavailable) {
        setUnavailable(true);
      } else {
        setError(res.error);
      }
    } catch {
      setError("Thêm combo vào giỏ hàng thất bại.");
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  if (unavailable) {
    return <p className="mt-2 text-text-disabled">Combo tạm không khả dụng</p>;
  }

  return (
    <div className="mt-2">
      {added ? (
        <p>
          Đã thêm combo.{" "}
          <Link href="/cart" className="underline">
            Xem giỏ hàng
          </Link>
        </p>
      ) : null}
      <button
        type="button"
        onClick={onClick}
        disabled={pending}
        className="mt-1 rounded-xs bg-action-primary px-3 py-1 text-text-inverse disabled:opacity-50"
      >
        Thêm combo vào giỏ
      </button>
      {error ? (
        <p role="alert" className="mt-1 text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
