"use client";

import { type FormEvent, useState } from "react";

import { Button } from "@/components/ui/Button";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { redeemReferralAction } from "./actions";

/**
 * Redeem someone else's referral code. A rejected code is reported twice: as an
 * error toast and as the FormItem help text (aria-invalid) under the field.
 */
export function RedeemReferralForm() {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void run(async () => {
      const res = await redeemReferralAction(code);
      if (res.ok) {
        setCode("");
        setError("");
        toast.success("Đã nhập mã giới thiệu.");
      } else {
        setError(res.error);
        toast.error(res.error);
      }
    });
  }

  return (
    <form
      onSubmit={submit}
      className="flex flex-col gap-3 sm:flex-row sm:items-start"
    >
      <FormItem
        label="Mã của bạn bè"
        help={error || undefined}
        status={error ? "error" : undefined}
        className="flex-1"
      >
        <Input
          className="min-h-10"
          name="code"
          value={code}
          onChange={(e) => {
            setCode(e.target.value);
            if (error) setError("");
          }}
          placeholder="Nhập mã của bạn bè"
          autoComplete="off"
        />
      </FormItem>
      <Button
        type="submit"
        isLoading={pending}
        disabled={!code.trim()}
        className="min-h-10 w-full sm:mt-6 sm:w-auto"
      >
        Nhập mã
      </Button>
    </form>
  );
}
