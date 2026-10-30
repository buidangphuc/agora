import type { ReactNode } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  ClosableTag,
  Dialog,
  Empty,
  Input,
  Link,
  Result,
  Select,
  Table,
  Tag,
  ToastProvider,
} from "./index";

const options = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
];

export const stateMatrix: Record<string, Record<string, ReactNode>> = {
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
  Tag: {
    default: <Tag>New</Tag>,
    success: <Tag tone="success">Paid</Tag>,
    danger: <Tag tone="danger">Cancelled</Tag>,
    closable: (
      <ClosableTag tone="primary" closeLabel="Remove filter Price">
        Price
      </ClosableTag>
    ),
  },
  Badge: {
    default: <Badge>3</Badge>,
    neutral: <Badge tone="neutral">12</Badge>,
    solid: (
      <Badge tone="danger" solid>
        Mall
      </Badge>
    ),
    pill: (
      <Badge pill size="md">
        2
      </Badge>
    ),
  },
  Empty: {
    default: <Empty description="No orders yet" />,
    action: (
      <Empty
        description="Your cart is empty"
        action={<Link href="/shop">Start shopping</Link>}
      />
    ),
  },
  Result: {
    success: (
      <Result status="success" title="Order placed" subTitle="Order 123" />
    ),
    error: <Result status="error" title="Payment failed" />,
    notFound: (
      <Result
        status="404"
        title="Page not found"
        extra={<Link href="/">Home</Link>}
      >
        <p>Check the address</p>
      </Result>
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
