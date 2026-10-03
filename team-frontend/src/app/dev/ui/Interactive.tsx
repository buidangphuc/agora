"use client";

import { useState } from "react";

import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Input,
  Modal,
  QuantityPicker,
  Radio,
  RadioGroup,
  Rate,
  Select,
  Statistic,
  Table,
  Tabs,
  Tag,
  Timeline,
  toast,
} from "@/components/ui";

/** Client-side demos for the /dev/ui catalogue: everything that holds state or takes callbacks. */

export function ModalDemo() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button data-testid="ui-modal-open" onClick={() => setOpen(true)}>
        Mở hộp thoại
      </Button>
      <Modal
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Chọn địa chỉ"
        description="Địa chỉ giao hàng của bạn"
        footer={
          <>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Huỷ
            </Button>
            <Button onClick={() => setOpen(false)}>Xác nhận</Button>
          </>
        }
      >
        <Input label="Họ tên" placeholder="Nguyễn Văn A" />
      </Modal>
    </>
  );
}

export function DrawerDemo() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant="outline"
        data-testid="ui-drawer-open"
        onClick={() => setOpen(true)}
      >
        Mở ngăn kéo
      </Button>
      <Drawer
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Bộ lọc"
        footer={<Button onClick={() => setOpen(false)}>Áp dụng</Button>}
      >
        <Input label="Giá tối đa" placeholder="1.000.000" />
      </Drawer>
    </>
  );
}

export function TabsDemo() {
  return (
    <div data-testid="ui-tabs">
      <Tabs
        defaultActiveId="all"
        items={[
          { id: "all", label: "Tất cả", content: <p>Tất cả đơn hàng</p> },
          {
            id: "shipping",
            label: "Đang giao",
            badge: 3,
            content: <p>Đơn đang giao</p>,
          },
          { id: "done", label: "Hoàn tất", content: <p>Đơn hoàn tất</p> },
        ]}
      />
    </div>
  );
}

export function PillsTabsDemo() {
  return (
    <Tabs
      variant="pills"
      defaultActiveId="a"
      items={[
        { id: "a", label: "Mô tả", content: <p>Mô tả sản phẩm</p> },
        { id: "b", label: "Đánh giá", content: <p>Đánh giá</p> },
      ]}
    />
  );
}

export function QuantityDemo() {
  const [value, setValue] = useState(3);
  return (
    <div className="flex items-center gap-3" data-testid="ui-quantity">
      <QuantityPicker min={1} max={3} value={value} onChange={setValue} />
      <span className="text-xs text-text-secondary">
        Giá trị: <b data-testid="ui-quantity-value">{value}</b> (tối đa 3)
      </span>
    </div>
  );
}

export function RateDemo() {
  const [value, setValue] = useState(0);
  return (
    <div className="flex items-center gap-3">
      <Rate value={value} onChange={setValue} />
      <span className="text-xs text-text-secondary">{value} sao</span>
    </div>
  );
}

export function FormControlsDemo() {
  const [city, setCity] = useState("");
  const [ship, setShip] = useState("std");
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Select
          aria-label="Tỉnh thành (controlled)"
          placeholder="Chọn tỉnh thành"
          value={city}
          onChange={(e) => setCity(e.target.value)}
          options={[
            { value: "hn", label: "Hà Nội" },
            { value: "hcm", label: "TP. Hồ Chí Minh" },
          ]}
        />
        <p className="text-xs text-text-secondary">Đã chọn: {city || "-"}</p>
      </div>
      <div className="space-y-1.5">
        <RadioGroup
          legend="Vận chuyển (controlled)"
          value={ship}
          onChange={setShip}
          options={[
            { value: "std", label: "Tiêu chuẩn" },
            { value: "fast", label: "Nhanh" },
          ]}
        />
        <Radio name="solo" label="Radio đơn" />
      </div>
    </div>
  );
}

export function ToastDemo() {
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="outline"
        onClick={() => toast.success("Đã lưu")}
      >
        Toast thành công
      </Button>
      <Button size="sm" variant="outline" onClick={() => toast.error("Có lỗi")}>
        Toast lỗi
      </Button>
      <Button
        size="sm"
        variant="outline"
        onClick={() => toast.info("Thông tin")}
      >
        Toast thông tin
      </Button>
    </div>
  );
}

export function ClosableDemo() {
  return (
    <div className="space-y-3">
      <Alert type="success" title="Đã lưu thay đổi" closable />
      <div className="flex gap-2">
        <Tag closable color="primary">
          Giá dưới 1 triệu
        </Tag>
        <Tag closable color="neutral">
          Freeship
        </Tag>
      </div>
    </div>
  );
}

/** Loading toggle: shows each data component switching between loading and loaded. */
export function LoadingToggleDemo() {
  const [loading, setLoading] = useState(true);
  return (
    <div className="space-y-3" data-testid="ui-loading-toggle">
      <Button
        size="sm"
        variant="outline"
        data-testid="ui-loading-toggle-button"
        onClick={() => setLoading((v) => !v)}
      >
        {loading ? "Hiện dữ liệu" : "Hiện skeleton"}
      </Button>
      <div className="grid gap-3 sm:grid-cols-3">
        <Statistic
          title="Doanh thu"
          value="12.500.000"
          prefix="₫"
          trend={{ value: "8%", isUp: true, label: "so với tuần trước" }}
          loading={loading}
        />
        <Statistic title="Đơn mới" value="42" loading={loading} />
        <Statistic
          title="Đánh giá"
          value="4.8"
          suffix="/ 5"
          loading={loading}
        />
      </div>
    </div>
  );
}

/** Error states whose retry needs a callback (client tree only). */
export function ErrorStatesDemo() {
  const [retries, setRetries] = useState(0);
  const retry = () => setRetries((n) => n + 1);
  return (
    <div className="space-y-4">
      <p className="text-xs text-text-secondary">
        Số lần thử lại: <b data-testid="ui-retry-count">{retries}</b>
      </p>
      <Table
        caption="Bảng lỗi"
        rowKey="id"
        dataSource={[]}
        columns={[{ key: "name", title: "Tên", dataIndex: "name" }]}
        error="Không tải được danh sách"
        onRetry={retry}
      />
      <Timeline items={[]} error="Không tải được lịch sử" onRetry={retry} />
      <Descriptions
        items={[]}
        error="Không tải được thông tin"
        onRetry={retry}
      />
      <Statistic
        title="Doanh thu"
        value=""
        error="Không tải được"
        onRetry={retry}
      />
    </div>
  );
}
