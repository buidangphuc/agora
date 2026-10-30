import type { ReactNode } from "react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Descriptions,
  Dialog,
  Input,
  Link,
  PageSkeleton,
  Progress,
  Select,
  Skeleton,
  Statistic,
  Table,
  ToastProvider,
} from "./index";

const options = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
];

const details = [
  { key: "name", label: "Name", children: "Desk lamp" },
  { key: "qty", label: "Qty", children: "2" },
];

export const stateMatrix: Record<string, Record<string, ReactNode>> = {
  Skeleton: {
    text: <Skeleton lines={2} />,
    avatar: <Skeleton variant="avatar" size="lg" />,
    image: <Skeleton variant="image" aspect="4/3" />,
    card: <Skeleton variant="card" />,
  },
  PageSkeleton: {
    default: <PageSkeleton />,
  },
  Progress: {
    default: <Progress label="Upload" percent={40} />,
    success: <Progress label="Upload" percent={100} status="success" />,
    danger: <Progress label="Upload" percent={60} status="danger" size="sm" />,
    hiddenInfo: <Progress label="Upload" percent={20} showInfo={false} />,
  },
  Descriptions: {
    default: <Descriptions title="Order" items={details} />,
    oneColumn: <Descriptions items={details} column={1} bordered={false} />,
    loading: <Descriptions items={details} loading />,
    empty: <Descriptions items={[]} emptyText="No details" />,
    fallback: <Descriptions items={details} fallback={<p>Failed</p>} />,
  },
  Statistic: {
    default: <Statistic title="Revenue" value="1,200" prefix="$" />,
    trend: (
      <Statistic
        title="Orders"
        value="12"
        trend={{ value: "5%", isUp: true, label: "vs last week" }}
      />
    ),
    danger: <Statistic title="Overdue" value="3" tone="danger" />,
    loading: (
      <Statistic title="Orders" value="12" trend={{ value: "5%" }} loading />
    ),
    fallback: <Statistic title="Orders" value="12" fallback={<p>Failed</p>} />,
  },
  Link: {
    default: <Link href="/shop">Back to shop</Link>,
  },
  Card: {
    default: <Card>Desk lamp</Card>,
    hoverable: <Card hoverable>Desk lamp</Card>,
    loading: <Card loading>Desk lamp</Card>,
    composed: (
      <Card padded={false}>
        <CardHeader>
          <CardTitle>Desk lamp</CardTitle>
        </CardHeader>
        <CardContent>
          <CardDescription>Warm light</CardDescription>
        </CardContent>
        <CardFooter>Free returns</CardFooter>
      </Card>
    ),
  },
  Button: {
    default: <Button>Save</Button>,
    secondary: <Button variant="secondary">Cancel</Button>,
    ghost: <Button variant="ghost">More</Button>,
    danger: <Button variant="danger">Delete</Button>,
    extraSmall: <Button size="xs">Save</Button>,
    fullWidth: <Button fullWidth>Save</Button>,
    fullWidthMobile: <Button fullWidth="mobile">Save</Button>,
    icons: (
      <Button
        leftIcon={<span aria-hidden="true">+</span>}
        rightIcon={<span aria-hidden="true">&gt;</span>}
      >
        Add
      </Button>
    ),
    disabled: <Button disabled>Save</Button>,
    loading: <Button loading>Saving</Button>,
  },
  Input: {
    default: <Input label="Name" />,
    help: <Input label="Name" helpText="As on your ID" />,
    error: <Input label="Name" errorText="Name is required" />,
    disabled: <Input label="Name" disabled />,
    required: <Input label="Name" required />,
  },
  Checkbox: {
    default: <Checkbox label="Accept" />,
    checked: <Checkbox label="Accept" defaultChecked />,
    error: <Checkbox label="Accept" errorText="Required" />,
    disabled: <Checkbox label="Accept" disabled />,
    indeterminate: <Checkbox label="Accept all" checked="indeterminate" />,
  },
  Select: {
    default: <Select label="Letter" options={options} />,
    selected: <Select label="Letter" options={options} defaultValue="a" />,
    error: <Select label="Letter" options={options} errorText="Pick one" />,
    disabled: <Select label="Letter" options={options} disabled />,
  },
  Dialog: {
    closed: (
      <Dialog triggerLabel="Open" title="Confirm" description="Are you sure?" />
    ),
    open: (
      <Dialog
        triggerLabel="Open"
        title="Confirm"
        description="Are you sure?"
        open
      />
    ),
  },
  Table: {
    rows: (
      <Table
        caption="Items"
        columns={[
          { key: "name", header: "Name" },
          { key: "qty", header: "Qty" },
        ]}
        rows={[{ id: 1, name: "Pen", qty: 2 }]}
      />
    ),
    empty: (
      <Table
        caption="Items"
        columns={[{ key: "name", header: "Name" }]}
        rows={[]}
      />
    ),
  },
  Toast: {
    provider: (
      <ToastProvider>
        <p>App</p>
      </ToastProvider>
    ),
  },
};
