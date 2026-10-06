"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { useToast } from "@/components/ui/ToastProvider";
import { upsertStorefrontAction } from "./actions";
import { SHOP_NAME_MAX, validateShopName } from "./shopName";
import { usePending } from "./usePending";

/**
 * Shop profile (Ant Basic Form): the shop display name, pre-filled with the
 * current one. Validates on blur and submit (trimmed, 1-80 characters), sends
 * nothing when invalid, and saves through upsertStorefrontAction.
 */
export function ShopProfileForm({ initialName }: { initialName: string }) {
  const [name, setName] = useState(initialName);
  const [touched, setTouched] = useState(false);
  const [serverError, setServerError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const invalid = validateShopName(name);
  const shown = touched ? invalid : null;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setTouched(true);
    setServerError("");
    if (invalid) return;
    const res = await run(() => upsertStorefrontAction(name));
    if (res.ok) {
      if (res.data) setName(res.data.displayName);
      toast.success("Đã lưu tên gian hàng");
    } else {
      setServerError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <Card>
      <CardContent>
        <form onSubmit={submit} noValidate className="max-w-xl space-y-4">
          <FormItem
            label="Tên gian hàng"
            required
            status={shown ? "error" : undefined}
            help={
              shown ??
              `Hiển thị trên trang gian hàng và trang sản phẩm. Tối đa ${SHOP_NAME_MAX} ký tự.`
            }
          >
            <Input
              id="shop-display-name"
              name="displayName"
              value={name}
              disabled={pending}
              onChange={(e) => setName(e.target.value)}
              onBlur={() => setTouched(true)}
              placeholder="VD: Nhà Sách An Nhiên"
            />
          </FormItem>
          {serverError && <Alert type="error" description={serverError} />}
          <Button type="submit" isLoading={pending}>
            Lưu thay đổi
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
