import type { ReactNode } from "react";

import {
  Alert,
  Avatar,
  Badge,
  Breadcrumb,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Descriptions,
  Empty,
  FormItem,
  Image,
  Input,
  Pagination,
  PriceTag,
  Progress,
  QuantityPicker,
  Radio,
  RadioGroup,
  Rate,
  Result,
  Select,
  Skeleton,
  Spin,
  Statistic,
  Stepper,
  Table,
  Tabs,
  Tag,
  Timeline,
} from "@/components/ui";

import {
  ClosableDemo,
  DrawerDemo,
  ErrorStatesDemo,
  FormControlsDemo,
  LoadingToggleDemo,
  ModalDemo,
  PillsTabsDemo,
  QuantityDemo,
  RateDemo,
  TabsDemo,
  ToastDemo,
} from "./Interactive";

function Section({
  id,
  title,
  children,
}: { id: string; title: string; children: ReactNode }) {
  return (
    <section id={id} data-testid={`ui-section-${id}`} className="space-y-4">
      <h2 className="border-b border-border-subtle pb-2 text-lg font-bold text-text-primary">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Demo({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium text-text-secondary">{label}</p>
      <div className="flex flex-wrap items-start gap-3">{children}</div>
    </div>
  );
}

interface Row {
  id: string;
  name: string;
  qty: number;
}
const rows: Row[] = [
  { id: "a", name: "Áo thun", qty: 2 },
  { id: "b", name: "Quần jean", qty: 5 },
];
const columns = [
  { key: "name", title: "Tên", dataIndex: "name" as const },
  {
    key: "qty",
    title: "SL",
    dataIndex: "qty" as const,
    align: "right" as const,
  },
];

const descItems = [
  { key: "a", label: "Họ tên", children: "Nguyễn Văn A" },
  { key: "b", label: "Trạng thái", children: "Đã xác minh" },
];

const timeline = [
  { key: "1", title: "Đã đặt hàng", time: "10:00", tone: "success" as const },
  {
    key: "2",
    title: "Đang giao",
    description: "Đơn vị vận chuyển đã nhận hàng",
    tone: "primary" as const,
    current: true,
  },
  { key: "3", title: "Giao thành công", tone: "neutral" as const },
];

const steps = [
  { id: 1, title: "Giỏ hàng", status: "complete" as const },
  { id: 2, title: "Thanh toán", status: "current" as const },
  { id: 3, title: "Hoàn tất", status: "upcoming" as const },
];

// 1x1 transparent GIF: a picture that always loads, no network.
const loadedPicture =
  "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

/** Every core component in every state (server-rendered; interactive demos are client islands). */
export function Catalogue() {
  return (
    <main className="mx-auto max-w-page space-y-12 px-4 py-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-bold text-text-primary">UI catalogue</h1>
        <p className="text-sm text-text-secondary">
          Every core component in every state. Development only: this route
          returns 404 in production builds.
        </p>
      </header>

      <Section id="button" title="Button">
        <Demo label="Variants">
          {(
            [
              "primary",
              "secondary",
              "outline",
              "ghost",
              "danger",
              "white",
            ] as const
          ).map((v) => (
            <Button key={v} variant={v} data-testid={`ui-button-${v}`}>
              {v}
            </Button>
          ))}
        </Demo>
        <Demo label="Sizes">
          {(["xs", "sm", "md", "lg"] as const).map((s) => (
            <Button key={s} size={s}>
              {s}
            </Button>
          ))}
        </Demo>
        <Demo label="Loading (width preserved) and disabled">
          <Button data-testid="ui-button-idle">Thêm vào giỏ</Button>
          <Button isLoading data-testid="ui-button-loading">
            Thêm vào giỏ
          </Button>
          <Button disabled data-testid="ui-button-disabled">
            Hết hàng
          </Button>
          <Button
            variant="outline"
            leftIcon={<span>★</span>}
            rightIcon={<span>→</span>}
          >
            Có biểu tượng
          </Button>
        </Demo>
      </Section>

      <Section id="data-entry" title="Data entry">
        <Demo label="Input: default, helper, error, disabled, required">
          <Input label="Email" placeholder="ban@example.com" />
          <Input label="Tên" helperText="Tối đa 50 ký tự" />
          <Input
            label="Email lỗi"
            error="Email không hợp lệ"
            defaultValue="abc"
          />
          <Input label="Mã" disabled defaultValue="Đã khoá" />
          <Input label="Họ tên" required />
        </Demo>
        <Demo label="Select: default, placeholder, error, disabled">
          <Select
            aria-label="Tỉnh"
            options={[
              { value: "hn", label: "Hà Nội" },
              { value: "hcm", label: "TP. Hồ Chí Minh" },
            ]}
          />
          <Select
            aria-label="Tỉnh 2"
            placeholder="Chọn tỉnh"
            options={[{ value: "hn", label: "Hà Nội" }]}
          />
          <Select aria-label="Tỉnh 3" invalid options={[]} />
          <Select
            aria-label="Tỉnh 4"
            disabled
            options={[{ value: "hn", label: "Hà Nội" }]}
          />
        </Demo>
        <Demo label="Checkbox and Radio">
          <Checkbox label="Nhớ tôi" />
          <Checkbox label="Đã chọn" defaultChecked />
          <Checkbox label="Vô hiệu" disabled />
          <RadioGroup
            legend="Giao hàng"
            defaultValue="std"
            options={[
              { value: "std", label: "Tiêu chuẩn" },
              { value: "fast", label: "Nhanh" },
              { value: "off", label: "Tạm tắt", disabled: true },
            ]}
          />
          <RadioGroup
            legend="Ngang"
            direction="horizontal"
            options={[
              { value: "a", label: "A" },
              { value: "b", label: "B" },
            ]}
          />
          <RadioGroup
            legend="Vô hiệu"
            disabled
            options={[{ value: "a", label: "A" }]}
          />
          <Radio name="single" label="Radio đơn" />
        </Demo>
        <Demo label="FormItem: required, help, error, success, group">
          <FormItem label="Tỉnh/Thành" required help="Chọn nơi nhận hàng">
            <Select options={[{ value: "hn", label: "Hà Nội" }]} />
          </FormItem>
          <FormItem label="Quận" status="error" help="Vui lòng chọn quận">
            <Select options={[]} />
          </FormItem>
          <FormItem label="Mã giảm giá" status="success" help="Mã hợp lệ">
            <Select options={[{ value: "x", label: "FREESHIP" }]} />
          </FormItem>
          <FormItem label="Vận chuyển" group help="Chọn một">
            <RadioGroup
              options={[
                { value: "a", label: "Nhanh" },
                { value: "b", label: "Tiết kiệm" },
              ]}
            />
          </FormItem>
        </Demo>
        <Demo label="QuantityPicker: at max (min 1, max 3), at min, disabled">
          <QuantityDemo />
          <QuantityPicker
            min={1}
            max={5}
            defaultValue={1}
            label="Số lượng (tối thiểu)"
          />
          <QuantityPicker
            defaultValue={2}
            disabled
            label="Số lượng (vô hiệu)"
          />
        </Demo>
        <Demo label="Rate: read-only, input, disabled">
          <Rate readOnly value={3.5} />
          <RateDemo />
          <Rate disabled defaultValue={2} label="Đánh giá (vô hiệu)" />
        </Demo>
        <Demo label="Controlled Select and RadioGroup">
          <FormControlsDemo />
        </Demo>
      </Section>

      <Section id="navigation" title="Navigation">
        <Demo label="Breadcrumb">
          <Breadcrumb
            items={[
              { label: "Trang chủ", href: "/" },
              { label: "Điện thoại", href: "/search?category=phones" },
              { label: "iPhone 15" },
            ]}
          />
        </Demo>
        <Demo label="Pagination (page 2 of 5) and a long range">
          <Pagination
            current={2}
            total={50}
            pageSize={10}
            hrefFor={(p) => `/dev/ui?page=${p}`}
          />
          <Pagination
            current={10}
            total={200}
            pageSize={10}
            hrefFor={(p) => `/dev/ui?page=${p}`}
          />
        </Demo>
        <Demo label="Tabs: client, pills, and link variant (server)">
          <TabsDemo />
          <PillsTabsDemo />
          <Tabs
            activeId="shipping"
            hrefFor={(id) => `/dev/ui?status=${id}`}
            items={[
              { id: "all", label: "Tất cả" },
              { id: "shipping", label: "Đang giao", badge: 3 },
              { id: "done", label: "Hoàn tất" },
            ]}
          />
        </Demo>
        <Demo label="Stepper: horizontal and vertical">
          <Stepper steps={steps} className="w-full" />
          <Stepper
            orientation="vertical"
            steps={[
              ...steps.slice(0, 2),
              { id: 3, title: "Lỗi thanh toán", status: "failed" as const },
            ]}
          />
        </Demo>
      </Section>

      <Section id="data-display" title="Data display">
        <Demo label="Badge variants">
          {(
            [
              "primary",
              "mall",
              "success",
              "warning",
              "danger",
              "neutral",
              "discount",
            ] as const
          ).map((v) => (
            <Badge key={v} variant={v}>
              {v}
            </Badge>
          ))}
          <Badge variant="primary" pill>
            3
          </Badge>
        </Demo>
        <Demo label="Tag presets and closable">
          {(
            [
              "neutral",
              "primary",
              "info",
              "success",
              "warning",
              "danger",
            ] as const
          ).map((c) => (
            <Tag key={c} color={c}>
              {c}
            </Tag>
          ))}
          <ClosableDemo />
        </Demo>
        <Demo label="Avatar: initials, failed picture, sizes">
          <Avatar name="Nguyễn Văn A" />
          <Avatar name="Trần Bình" src="/dev-ui-missing-avatar.png" />
          <Avatar name="Lê" size="sm" />
          <Avatar name="Phạm Dũng" size="xl" shape="square" />
        </Demo>
        <Demo label="Image: loaded, failed (fallback), fixed aspect boxes">
          <div className="w-32" data-testid="ui-image-ok">
            <Image src={loadedPicture} alt="Ảnh mẫu" aspect="square" />
          </div>
          <div className="w-32" data-testid="ui-image-broken">
            <Image
              src="/dev-ui-missing-image.png"
              alt="Ảnh hỏng"
              aspect="square"
            />
          </div>
          <div className="w-48">
            <Image src="/dev-ui-missing-image.png" alt="Ảnh 4:3" aspect="4/3" />
          </div>
        </Demo>
        <Demo label="PriceTag sizes">
          <PriceTag price={29990000} size="sm" />
          <PriceTag price={29990000} originalPrice={34990000} size="md" />
          <PriceTag price={29990000} originalPrice={34990000} size="lg" />
          <PriceTag price={29990000} originalPrice={34990000} size="xl" />
        </Demo>
        <Demo label="Card: default, hoverable, loading">
          <Card className="w-64">
            <CardHeader>
              <CardTitle>Tiêu đề</CardTitle>
            </CardHeader>
            <CardContent>
              <CardDescription>Nội dung thẻ</CardDescription>
            </CardContent>
            <CardFooter>Chân thẻ</CardFooter>
          </Card>
          <Card hoverable className="w-64">
            <CardContent>Thẻ có hiệu ứng hover</CardContent>
          </Card>
          <Card loading className="w-64">
            <CardContent>Không hiển thị</CardContent>
          </Card>
        </Demo>
        <Demo label="Descriptions: default, loading, empty">
          <Descriptions
            title="Thông tin"
            items={descItems}
            className="w-full"
          />
          <Descriptions
            title="Đang tải"
            items={descItems}
            loading
            className="w-full"
          />
          <Descriptions
            title="Trống"
            items={[]}
            emptyText="Chưa có thông tin"
            className="w-full"
          />
        </Demo>
        <Demo label="Statistic: value, loading toggle">
          <Statistic
            title="Doanh thu"
            value="12.500.000"
            prefix="₫"
            trend={{ value: "8%", isUp: true, label: "so với tuần trước" }}
          />
          <Statistic
            title="Tỉ lệ huỷ"
            value="2.1"
            suffix="%"
            trend={{ value: "0.4%", isUp: false }}
          />
        </Demo>
        <LoadingToggleDemo />
        <Demo label="Table: data, empty, loading">
          <Table
            caption="Sản phẩm"
            columns={columns}
            dataSource={rows}
            rowKey="id"
            className="w-full"
          />
          <Table
            caption="Bảng trống"
            columns={columns}
            dataSource={[]}
            rowKey="id"
            emptyText="Chưa có sản phẩm"
            className="w-full"
          />
          <Table
            caption="Bảng đang tải"
            columns={columns}
            dataSource={rows}
            rowKey="id"
            loading
            loadingRows={3}
            className="w-full"
          />
        </Demo>
        <Demo label="Timeline: default, loading, empty">
          <Timeline items={timeline} />
          <Timeline items={timeline} loading className="w-64" />
          <Timeline items={[]} emptyText="Chưa có cập nhật" />
        </Demo>
        <Demo label="Skeleton presets">
          <Skeleton variant="text" lines={3} className="w-48" />
          <Skeleton variant="avatar" />
          <Skeleton variant="image" aspect="4/3" className="w-48" />
          <Skeleton variant="card" className="w-48" />
        </Demo>
        <Demo label="Empty: default and with action">
          <Empty />
          <Empty
            description="Giỏ hàng trống"
            action={<Button size="sm">Mua sắm ngay</Button>}
          />
        </Demo>
        <Demo label="Result statuses">
          {(["success", "error", "info", "warning", "404"] as const).map(
            (s) => (
              <Result
                key={s}
                status={s}
                title={`Trạng thái ${s}`}
                subTitle="Mô tả ngắn"
                className="w-72"
              />
            ),
          )}
        </Demo>
      </Section>

      <Section id="feedback" title="Feedback">
        <Demo label="Alert types, with action, closable">
          <div className="w-full space-y-3">
            <Alert
              type="info"
              title="Thông tin"
              description="Nội dung thông tin"
            />
            <Alert type="success" title="Thành công" />
            <Alert
              type="warning"
              title="Cảnh báo"
              description="Sắp hết hàng"
              action={
                <a
                  href="/dev/ui"
                  className="text-sm font-medium text-action-primary"
                >
                  Xem
                </a>
              }
            />
            <Alert
              type="error"
              title="Lỗi"
              description="Không thể thanh toán"
            />
          </div>
        </Demo>
        <Demo label="Spin: standalone and wrapping content">
          <Spin />
          <Spin tip="Đang xử lý" size="lg" />
          <Spin spinning>
            <p className="p-6 text-sm">Nội dung bị phủ</p>
          </Spin>
        </Demo>
        <Demo label="Progress">
          <div className="w-full space-y-3">
            <Progress percent={30} label="Đang tải" />
            <Progress percent={100} status="success" label="Hoàn tất" />
            <Progress percent={60} status="danger" size="sm" label="Lỗi" />
          </div>
        </Demo>
        <Demo label="Modal, Drawer and Toast">
          <ModalDemo />
          <DrawerDemo />
          <ToastDemo />
        </Demo>
        <Demo label="Error states with retry (client)">
          <div className="w-full">
            <ErrorStatesDemo />
          </div>
        </Demo>
      </Section>
    </main>
  );
}
