"use client";

import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { createCampaignAction } from "../actions";
import { usePending } from "../usePending";
import { ListingPicker, type PickedListing } from "./ListingPicker";

type Field = "listing" | "price" | "cap" | "start" | "end";

const EMPTY = { price: "", cap: "", start: "", end: "" };

export function FlashSaleForm() {
  const router = useRouter();
  const [listing, setListing] = useState<PickedListing | null>(null);
  const [f, setF] = useState(EMPTY);
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({});
  const [serverError, setServerError] = useState("");
  const { pending, run } = usePending();
  const inFlight = useRef(false);
  const key = useRef("");
  const set = (k: keyof typeof EMPTY) => (e: { target: { value: string } }) =>
    setF((p) => ({ ...p, [k]: e.target.value }));

  function validate() {
    const errs: Partial<Record<Field, string>> = {};
    const price = Number(f.price);
    const cap = Number(f.cap);
    const start = new Date(f.start);
    const end = new Date(f.end);
    if (!listing) errs.listing = "Chọn sản phẩm cho flash sale";
    if (!(price > 0)) errs.price = "Giá flash sale phải lớn hơn 0";
    if (!Number.isInteger(cap) || cap < 1) {
      errs.cap = "Số lượng tối đa phải từ 1 trở lên";
    }
    if (Number.isNaN(start.getTime())) errs.start = "Chọn thời gian bắt đầu";
    if (Number.isNaN(end.getTime())) errs.end = "Chọn thời gian kết thúc";
    else if (!errs.start && end <= start) {
      errs.end = "Thời gian kết thúc phải sau thời gian bắt đầu";
    }
    return errs;
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (inFlight.current) return;
    const errs = validate();
    setErrors(errs);
    setServerError("");
    if (Object.keys(errs).length > 0 || !listing) return;
    if (!key.current) key.current = crypto.randomUUID();
    inFlight.current = true;
    try {
      const res = await run(() =>
        createCampaignAction({
          listingId: listing.id,
          salePrice: Number(f.price),
          stockCap: Number(f.cap),
          startsAt: new Date(f.start),
          endsAt: new Date(f.end),
          idempotencyKey: key.current,
        }),
      );
      if (res.ok) {
        key.current = "";
        setF(EMPTY);
        setListing(null);
        router.refresh();
      } else {
        setServerError(res.error);
      }
    } finally {
      inFlight.current = false;
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <fieldset disabled={pending} className="m-0 space-y-4 border-0 p-0">
        <ListingPicker onSelect={setListing} />
        {listing && <p className="text-sm">Đã chọn: {listing.title}</p>}
        {errors.listing && (
          <p role="alert" className="text-sm text-danger">
            {errors.listing}
          </p>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Input
            label="Giá flash sale (₫)"
            required
            type="number"
            min={0}
            value={f.price}
            error={errors.price}
            onChange={set("price")}
          />
          <Input
            label="Số lượng tối đa"
            required
            type="number"
            min={1}
            value={f.cap}
            error={errors.cap}
            onChange={set("cap")}
          />
          <Input
            label="Bắt đầu"
            required
            type="datetime-local"
            value={f.start}
            error={errors.start}
            onChange={set("start")}
          />
          <Input
            label="Kết thúc"
            required
            type="datetime-local"
            value={f.end}
            error={errors.end}
            onChange={set("end")}
          />
        </div>
      </fieldset>
      {Object.keys(errors).length > 0 && (
        <p role="alert" className="text-sm text-danger">
          Vui lòng kiểm tra lại các trường được đánh dấu.
        </p>
      )}
      {serverError && <Alert type="error" description={serverError} />}
      <Button type="submit" isLoading={pending}>
        Tạo flash sale
      </Button>
    </form>
  );
}
