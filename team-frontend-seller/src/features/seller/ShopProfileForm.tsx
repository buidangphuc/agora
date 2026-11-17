"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { FeaturedListingsPicker } from "./FeaturedListingsPicker";
import { ShopBannerField } from "./ShopBannerField";
import { upsertStorefrontAction } from "./actions";
import { THEMES, validateTagline } from "./shopBranding";
import { SHOP_NAME_MAX, validateShopName } from "./shopName";
import { usePending } from "./usePending";

export interface ShopProfileInitial {
  displayName: string;
  bannerUrl: string;
  tagline: string;
  featuredListingIds: string[];
  theme: string;
}

const THEME_LABELS: Record<string, string> = {
  default: "Mặc định",
  dark: "Tối",
  festive: "Lễ hội",
};

/**
 * Shop profile (Ant Basic Form): name, banner, tagline, featured listings and
 * theme, pre-filled from the storefront. Validates on blur and submit, sends
 * nothing when invalid, and saves the full storefront through
 * upsertStorefrontAction.
 */
export function ShopProfileForm({
  initial,
  titles,
}: {
  initial: ShopProfileInitial;
  titles: Record<string, string>;
}) {
  const [name, setName] = useState(initial.displayName);
  const [bannerUrl, setBannerUrl] = useState(initial.bannerUrl);
  const [tagline, setTagline] = useState(initial.tagline);
  const [featured, setFeatured] = useState(initial.featuredListingIds);
  const [theme, setTheme] = useState(initial.theme || "default");
  const [touched, setTouched] = useState(false);
  const [serverError, setServerError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const invalid = validateShopName(name);
  const shown = touched ? invalid : null;
  const taglineInvalid = validateTagline(tagline);
  const taglineShown = touched ? taglineInvalid : null;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setTouched(true);
    setServerError("");
    if (invalid || taglineInvalid) return;
    const res = await run(() =>
      upsertStorefrontAction({
        displayName: name,
        bannerUrl,
        tagline,
        featuredListingIds: featured,
        theme,
      }),
    );
    if (res.ok) {
      if (res.data) setName(res.data.displayName);
      toast.success("Đã lưu thông tin gian hàng");
    } else {
      setServerError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <Card>
      <CardContent>
        <form onSubmit={submit} noValidate className="max-w-xl space-y-4">
          <Input
            required
            label="Tên gian hàng"
            error={shown ?? undefined}
            helperText={
              shown
                ? undefined
                : `Hiển thị trên trang gian hàng và trang sản phẩm. Tối đa ${SHOP_NAME_MAX} ký tự.`
            }
            id="shop-display-name"
            name="displayName"
            value={name}
            disabled={pending}
            onChange={(e) => setName(e.target.value)}
            onBlur={() => setTouched(true)}
            placeholder="VD: Nhà Sách An Nhiên"
          />
          <ShopBannerField value={bannerUrl} onChange={setBannerUrl} />
          <Input
            label="Tagline"
            error={taglineShown ?? undefined}
            id="shop-tagline"
            name="tagline"
            value={tagline}
            disabled={pending}
            onChange={(e) => setTagline(e.target.value)}
            onBlur={() => setTouched(true)}
          />
          <FeaturedListingsPicker
            value={featured}
            titles={titles}
            onChange={setFeatured}
          />
          <Select
            label="Giao diện"
            id="shop-theme"
            name="theme"
            value={theme}
            disabled={pending}
            options={THEMES.map((t) => ({
              value: t,
              label: THEME_LABELS[t] ?? t,
            }))}
            onChange={(e) => setTheme(e.target.value)}
          />
          {serverError && <Alert type="error" description={serverError} />}
          <Button type="submit" isLoading={pending}>
            Lưu thay đổi
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
