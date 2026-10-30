import type { ReactNode } from "react";
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Dialog,
  Input,
  Link,
  Select,
  Table,
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
  Toast: {
    provider: (
      <ToastProvider>
        <p>App</p>
      </ToastProvider>
    ),
    top: (
      <ToastProvider position="top" label="Thông báo" dismissLabel="Đóng">
        <p>App</p>
      </ToastProvider>
    ),
  },
  Alert: {
    info: <Alert tone="info" title="Heads up" />,
    success: <Alert tone="success" description="Saved" />,
    warning: (
      <Alert tone="warning" title="Low stock" description="2 items left" />
    ),
    danger: <Alert tone="danger" title="Payment failed" />,
    action: (
      <Alert
        tone="danger"
        title="Payment failed"
        action={<Button size="sm">Retry</Button>}
      />
    ),
    closable: <Alert tone="success" title="Saved" closable />,
  },
};
