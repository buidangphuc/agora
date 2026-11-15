"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { createAdCampaignAction } from "./actions";
import { usePending } from "./usePending";

interface SellerListingOption {
  id: string;
  title: string;
}

type Field = "listing" | "budget" | "bid";

/** Client-side rules, the same texts as createAdCampaignAction. */
export function validateAdCampaign(
  listingId: string,
  budget: number,
  bid: number,
): Partial<Record<Field, string>> {
  const errors: Partial<Record<Field, string>> = {};
  if (!listingId) errors.listing = "Chọn sản phẩm cần quảng cáo.";
  if (!(budget > 0)) errors.budget = "Ngân sách phải lớn hơn 0.";
  if (!(bid > 0)) errors.bid = "Giá thầu phải lớn hơn 0.";
  else if (bid < 500) errors.bid = "Giá thầu tối thiểu là ₫500.";
  else if (budget > 0 && budget < bid) {
    errors.budget = "Ngân sách phải lớn hơn hoặc bằng giá thầu.";
  }
  return errors;
}

/**
 * Sponsored-ad campaign form (Ant Basic Form): pick a listing, set budget and
 * bid, launch. Wired to team-promotion through createAdCampaignAction.
 */
export function AdCampaignForm({
  listings,
}: {
  listings: SellerListingOption[];
}) {
  const [listingId, setListingId] = useState(listings[0]?.id ?? "");
  const [budget, setBudget] = useState("");
  const [bid, setBid] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const errors = validateAdCampaign(listingId, Number(budget), Number(bid));

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitted(true);
    setError("");
    if (Object.keys(errors).length > 0) return;
    const res = await run(() =>
      createAdCampaignAction(listingId, Number(budget), Number(bid)),
    );
    if (res.ok) {
      setBudget("");
      setBid("");
      setSubmitted(false);
      toast.success("Đã tạo chiến dịch quảng cáo");
    } else if (!res.ok) {
      setError(res.error);
      toast.error(res.error);
    }
  }

  const shown = (field: Field) => (submitted ? errors[field] : undefined);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tạo chiến dịch mới</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} noValidate className="space-y-4">
            <fieldset disabled={pending} className="m-0 space-y-4 border-0 p-0">
              <Select
                label="Sản phẩm"
                required
                error={shown("listing") || undefined}
                value={listingId}
                onChange={(e) => setListingId(e.target.value)}
                placeholder={
                  listings.length === 0 ? "Shop chưa có sản phẩm" : undefined
                }
                options={listings.map((l) => ({
                  value: l.id,
                  label: l.title,
                }))}
              />
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Input
                  required
                  label="Ngân sách (₫)"
                  error={shown("budget")}
                  type="number"
                  min={0}
                  value={budget}
                  onChange={(e) => setBudget(e.target.value)}
                />
                <Input
                  required
                  label="Giá thầu mỗi lượt (₫)"
                  error={shown("bid")}
                  type="number"
                  min={0}
                  value={bid}
                  onChange={(e) => setBid(e.target.value)}
                />
              </div>
            </fieldset>
            {error && <Alert type="error" description={error} />}
            <Button type="submit" isLoading={pending}>
              Chạy quảng cáo
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
