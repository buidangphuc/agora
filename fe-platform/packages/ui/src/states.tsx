import type { ReactNode } from "react";
import {
  Breadcrumb,
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
  Pagination,
  Select,
  Stepper,
  Table,
  Tabs,
  Timeline,
  ToastProvider,
} from "./index";

const tabItems = [
  { id: "all", label: "All", content: <p>All orders</p> },
  { id: "open", label: "Open", badge: 3, content: <p>Open orders</p> },
  { id: "done", label: "Done", disabled: true },
];

const steps = [
  { id: 1, title: "Cart", status: "complete" as const, href: "/cart" },
  { id: 2, title: "Payment", status: "current" as const },
  { id: 3, title: "Done", status: "upcoming" as const },
];

const options = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
];

export const stateMatrix: Record<string, Record<string, ReactNode>> = {
  Tabs: {
    underline: <Tabs items={tabItems} />,
    pills: <Tabs items={tabItems} variant="pills" activeId="open" />,
    links: (
      <Tabs
        items={tabItems}
        activeId="open"
        hrefFor={(id) => `/orders/${id}`}
      />
    ),
  },
  Pagination: {
    first: <Pagination current={1} total={50} hrefFor={(p) => `/?page=${p}`} />,
    middle: (
      <Pagination current={10} total={200} hrefFor={(p) => `/?page=${p}`} />
    ),
    last: <Pagination current={5} total={50} hrefFor={(p) => `/?page=${p}`} />,
  },
  Breadcrumb: {
    default: (
      <Breadcrumb
        items={[
          { label: "Home", href: "/" },
          { label: "Phones", href: "/phones" },
          { label: "Pixel" },
        ]}
      />
    ),
  },
  Stepper: {
    horizontal: <Stepper steps={steps} />,
    vertical: (
      <Stepper
        orientation="vertical"
        steps={[
          ...steps.slice(0, 2),
          { id: 3, title: "Failed", status: "failed", description: "Retry" },
        ]}
      />
    ),
    inert: (
      <Stepper steps={[{ ...steps[0], disabled: true }, ...steps.slice(1)]} />
    ),
  },
  Timeline: {
    default: (
      <Timeline
        items={[
          { key: "1", title: "Ordered", time: "10:00", tone: "success" },
          {
            key: "2",
            title: "Shipping",
            description: "With the carrier",
            tone: "primary",
            current: true,
          },
        ]}
      />
    ),
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
