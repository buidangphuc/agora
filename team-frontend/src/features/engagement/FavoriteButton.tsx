"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { addFavoriteAction, removeFavoriteAction } from "./actions";

// Subtle heart toggle matching Shopee aesthetics
export function FavoriteButton({
  id,
  initial,
}: { id: string; initial: boolean }) {
  const [fav, setFav] = useState(initial);
  const [pending, start] = useTransition();
  const toast = useToast();

  function toggle(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    const next = !fav;
    setFav(next);
    if (next) {
      toast.success("✓ Đã thêm sản phẩm vào mục Yêu Thích!");
    } else {
      toast.info("Đã bỏ thích sản phẩm.");
    }
    start(async () => {
      try {
        if (next) await addFavoriteAction(id);
        else await removeFavoriteAction(id);
      } catch {
        setFav(!next);
        toast.error("Có lỗi xảy ra khi cập nhật yêu thích.");
      }
    });
  }

  return (
    <button
      type="button"
      onClick={toggle}
      disabled={pending}
      aria-label={fav ? "Bỏ thích" : "Yêu thích"}
      className={`grid h-7 w-7 place-items-center rounded-full transition shadow-xs ${
        fav
          ? "bg-red-500 text-white"
          : "bg-black/30 text-white hover:bg-black/50 backdrop-blur-xs"
      }`}
    >
      <span className="text-xs leading-none">{fav ? "♥" : "♡"}</span>
    </button>
  );
}
