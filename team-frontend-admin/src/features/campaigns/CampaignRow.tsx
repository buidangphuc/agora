"use client";

import { type FormEvent, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Badge, type BadgeVariant } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { formatPrice } from "@/components/ui/format";
import { formatCampaignTime } from "@/features/admin/campaignForm";
import type { CampaignStatusName, CampaignView } from "@/lib/gateway/campaigns";
import { usePending } from "@/lib/usePending";

import { cancelCampaignAction, updateCampaignAction } from "./actions";
import {
  type CampaignEditErrors,
  type CampaignEditValues,
  toLocalInput,
} from "./campaignForm";

const STATUS_LABEL: Record<CampaignStatusName, string> = {
  scheduled: "Sắp chạy",
  active: "Đang chạy",
  ended: "Đã kết thúc",
  cancelled: "Đã huỷ",
  unspecified: "Không rõ",
};

const STATUS_VARIANT: Record<CampaignStatusName, BadgeVariant> = {
  scheduled: "primary",
  active: "success",
  ended: "neutral",
  cancelled: "danger",
  unspecified: "neutral",
};

const EDIT_FIELDS: {
  key: keyof CampaignEditValues;
  label: string;
  type: "number" | "datetime-local";
}[] = [
  { key: "salePrice", label: "Giá flash sale (₫)", type: "number" },
  { key: "stockCap", label: "Số lượng tối đa", type: "number" },
  { key: "startsAt", label: "Bắt đầu (giờ Việt Nam)", type: "datetime-local" },
  { key: "endsAt", label: "Kết thúc (giờ Việt Nam)", type: "datetime-local" },
];

function editValues(c: CampaignView): CampaignEditValues {
  return {
    salePrice: String(c.salePrice),
    stockCap: String(c.stockCap),
    startsAt: toLocalInput(c.startsAt),
    endsAt: toLocalInput(c.endsAt),
  };
}

export function CampaignRow({ campaign: initial }: { campaign: CampaignView }) {
  const [campaign, setCampaign] = useState(initial);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [values, setValues] = useState<CampaignEditValues>(editValues(initial));
  const [fieldErrors, setFieldErrors] = useState<CampaignEditErrors>({});
  const [error, setError] = useState("");
  const { pending, run } = usePending();

  const cancellable =
    campaign.status === "scheduled" || campaign.status === "active";
  const editable = campaign.status === "scheduled";

  function closeCancel() {
    if (pending) return;
    setError("");
    setCancelOpen(false);
  }

  function closeEdit() {
    if (pending) return;
    setError("");
    setFieldErrors({});
    setEditOpen(false);
  }

  async function onCancel() {
    if (pending) return;
    setError("");
    const res = await run(() => cancelCampaignAction(campaign.id));
    if (res.ok) {
      setCampaign(res.campaign);
      setCancelOpen(false);
      return;
    }
    setError(res.error);
  }

  async function onEdit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (pending) return;
    setError("");
    setFieldErrors({});
    const res = await run(() => updateCampaignAction(campaign.id, values));
    if (res.ok) {
      setCampaign(res.campaign);
      setValues(editValues(res.campaign));
      setEditOpen(false);
      return;
    }
    setError(res.error);
    if (res.fieldErrors) setFieldErrors(res.fieldErrors as CampaignEditErrors);
  }

  return (
    <tr data-testid="campaign-row">
      <td>{campaign.id}</td>
      <td>{campaign.listingId}</td>
      <td>{formatPrice(campaign.salePrice)}</td>
      <td>{`${campaign.stockSold} / ${campaign.stockCap}`}</td>
      <td>
        {`${formatCampaignTime(campaign.startsAt)} – ${formatCampaignTime(campaign.endsAt)}`}
      </td>
      <td>
        <span data-testid="campaign-status" data-status={campaign.status}>
          <Badge variant={STATUS_VARIANT[campaign.status]}>
            {STATUS_LABEL[campaign.status]}
          </Badge>
        </span>
      </td>
      <td>
        <div className="flex gap-2">
          {editable ? (
            <Button
              variant="secondary"
              size="sm"
              data-testid="campaign-edit"
              onClick={() => setEditOpen(true)}
            >
              Sửa
            </Button>
          ) : null}
          {cancellable ? (
            <Button
              variant="danger"
              size="sm"
              data-testid="campaign-cancel"
              onClick={() => setCancelOpen(true)}
            >
              Huỷ
            </Button>
          ) : null}
        </div>
        <Modal
          isOpen={cancelOpen}
          onClose={closeCancel}
          size="sm"
          title="Huỷ flash sale"
          description={`Chiến dịch ${campaign.id} sẽ dừng ngay và không hoàn tác được.`}
          footer={
            <>
              <Button
                variant="secondary"
                onClick={closeCancel}
                disabled={pending}
              >
                Quay lại
              </Button>
              <Button
                variant="danger"
                data-testid="campaign-cancel-confirm"
                onClick={onCancel}
                isLoading={pending}
                disabled={pending}
              >
                Xác nhận huỷ
              </Button>
            </>
          }
        >
          {error ? <Alert type="error" description={error} /> : null}
        </Modal>
        <Modal
          isOpen={editOpen}
          onClose={closeEdit}
          size="sm"
          title="Sửa flash sale"
          footer={
            <>
              <Button
                variant="secondary"
                onClick={closeEdit}
                disabled={pending}
              >
                Quay lại
              </Button>
              <Button
                type="submit"
                form={`campaign-edit-${campaign.id}`}
                isLoading={pending}
                disabled={pending}
              >
                Lưu thay đổi
              </Button>
            </>
          }
        >
          <form
            id={`campaign-edit-${campaign.id}`}
            noValidate
            onSubmit={onEdit}
            className="flex flex-col gap-4"
          >
            {EDIT_FIELDS.map((f) => (
              <Input
                key={f.key}
                name={f.key}
                label={f.label}
                type={f.type}
                value={values[f.key]}
                onChange={(e) =>
                  setValues((v) => ({ ...v, [f.key]: e.target.value }))
                }
                error={fieldErrors[f.key]}
              />
            ))}
            {error ? <Alert type="error" description={error} /> : null}
          </form>
        </Modal>
      </td>
    </tr>
  );
}
