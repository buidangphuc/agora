"use client";

import { type FormEvent, useState } from "react";

import { LinkButton } from "@/components/LinkButton";
import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Descriptions } from "@/components/ui/Descriptions";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { formatPrice } from "@/components/ui/format";
import type {
  CampaignView,
  CreateCampaignInput,
} from "@/lib/gateway/campaigns";
import { usePending } from "@/lib/usePending";

import { CampaignDetails } from "./CampaignDetails";
import { createCampaignAction } from "./campaignActions";
import {
  type CampaignDraft,
  type CampaignField,
  type CampaignFieldErrors,
  EMPTY_DRAFT,
  formatCampaignTime,
  parseCampaignDraft,
} from "./campaignForm";

const FIELDS: {
  key: CampaignField;
  label: string;
  type: "text" | "number" | "datetime-local";
  required: boolean;
  help?: string;
}[] = [
  { key: "listingId", label: "Mã tin đăng", type: "text", required: true },
  {
    key: "variantId",
    label: "Mã phân loại",
    type: "text",
    required: false,
    help: "Để trống để áp dụng cho mọi phân loại của tin.",
  },
  {
    key: "salePrice",
    label: "Giá flash sale (₫)",
    type: "number",
    required: true,
  },
  {
    key: "stockCap",
    label: "Số lượng tối đa",
    type: "number",
    required: true,
  },
  {
    key: "startsAt",
    label: "Bắt đầu (giờ Việt Nam)",
    type: "datetime-local",
    required: true,
  },
  {
    key: "endsAt",
    label: "Kết thúc (giờ Việt Nam)",
    type: "datetime-local",
    required: true,
  },
];

function Summary({ value }: { value: CreateCampaignInput }) {
  return (
    <Descriptions
      data-testid="campaign-confirm-summary"
      column={1}
      items={[
        { key: "listing", label: "Mã tin đăng", children: value.listingId },
        {
          key: "variant",
          label: "Mã phân loại",
          children: value.variantId || "Tất cả phân loại",
        },
        {
          key: "price",
          label: "Giá flash sale",
          children: formatPrice(value.salePrice),
        },
        { key: "stock", label: "Số lượng tối đa", children: value.stockCap },
        {
          key: "starts",
          label: "Bắt đầu",
          children: formatCampaignTime(value.startsAt.toISOString()),
        },
        {
          key: "ends",
          label: "Kết thúc",
          children: formatCampaignTime(value.endsAt.toISOString()),
        },
      ]}
    />
  );
}

export function CreateCampaignForm() {
  const [draft, setDraft] = useState<CampaignDraft>(EMPTY_DRAFT);
  const [fieldErrors, setFieldErrors] = useState<CampaignFieldErrors>({});
  const [review, setReview] = useState<CreateCampaignInput | null>(null);
  const [error, setError] = useState("");
  const [warning, setWarning] = useState("");
  const [created, setCreated] = useState<CampaignView | null>(null);
  const { pending, run } = usePending();

  function onReview(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setWarning("");
    const parsed = parseCampaignDraft(draft);
    if (!parsed.ok) {
      setFieldErrors(parsed.errors);
      return;
    }
    setFieldErrors({});
    setError("");
    setReview(parsed.value);
  }

  function closeReview() {
    if (pending) return;
    setError("");
    setReview(null);
  }

  async function onConfirm() {
    if (pending || !review) return;
    setError("");
    const res = await run(() => createCampaignAction(draft));
    if (res.ok) {
      setCreated(res.campaign);
      setReview(null);
      setDraft(EMPTY_DRAFT);
      return;
    }
    if (res.fieldErrors) setFieldErrors(res.fieldErrors);
    if (res.uncertain || res.fieldErrors) {
      setReview(null);
      setWarning(res.error);
      return;
    }
    setError(res.error);
  }

  if (created) {
    return (
      <div className="space-y-4">
        <Alert
          type="success"
          data-testid="campaign-created-status"
          description="Đã tạo chiến dịch flash sale. Thông tin dưới đây do hệ thống trả về."
        />
        <CampaignDetails
          campaign={created}
          title="Chiến dịch vừa tạo"
          testId="campaign-created"
        />
        <div className="flex flex-wrap gap-3">
          <LinkButton href="/admin/campaigns">
            Xem chiến dịch đang chạy
          </LinkButton>
          <Button variant="secondary" onClick={() => setCreated(null)}>
            Tạo chiến dịch khác
          </Button>
        </div>
      </div>
    );
  }

  return (
    <>
      <form
        noValidate
        onSubmit={onReview}
        data-testid="campaign-form"
        className="grid grid-cols-1 gap-4 md:grid-cols-2"
      >
        {FIELDS.map((f) => (
          <Input
            key={f.key}
            name={f.key}
            label={f.label}
            type={f.type}
            required={f.required}
            min={f.type === "number" ? 1 : undefined}
            step={f.type === "number" ? 1 : undefined}
            inputMode={f.type === "number" ? "numeric" : undefined}
            value={draft[f.key]}
            onChange={(e) =>
              setDraft((d) => ({ ...d, [f.key]: e.target.value }))
            }
            helperText={f.help}
            error={fieldErrors[f.key]}
          />
        ))}
        <div className="md:col-span-2 space-y-3">
          {warning ? (
            <Alert
              type="warning"
              data-testid="campaign-form-warning"
              description={warning}
            />
          ) : null}
          <Button type="submit">Xem lại trước khi tạo</Button>
        </div>
      </form>

      <Modal
        isOpen={review !== null}
        onClose={closeReview}
        size="sm"
        title="Xác nhận tạo flash sale"
        description="Chiến dịch áp dụng cho người mua trên toàn sàn và chưa thể huỷ hay sửa sau khi tạo."
        footer={
          <>
            <Button variant="outline" onClick={closeReview} disabled={pending}>
              Quay lại sửa
            </Button>
            <Button onClick={onConfirm} isLoading={pending} disabled={pending}>
              Xác nhận tạo
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          {review ? <Summary value={review} /> : null}
          {error ? (
            <Alert
              type="error"
              data-testid="campaign-confirm-error"
              description={error}
            />
          ) : null}
        </div>
      </Modal>
    </>
  );
}
