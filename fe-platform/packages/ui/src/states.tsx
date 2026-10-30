import type { ReactNode } from "react";
import {
  Avatar,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Dialog,
  Drawer,
  Image,
  Input,
  Link,
  PriceTag,
  Select,
  Table,
  ToastProvider,
} from "./index";

const options = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
];

export const stateMatrix: Record<string, Record<string, ReactNode>> = {
  PriceTag: {
    default: <PriceTag price={29990000} />,
    discounted: (
      <PriceTag price={29990000} originalPrice={34990000} size="xl" />
    ),
    noSymbol: <PriceTag price={1000} size="sm" showCurrencySymbol={false} />,
  },
  Image: {
    default: <Image src="/lamp.jpg" alt="Desk lamp" aspect="square" />,
    empty: <Image src="" alt="Desk lamp" aspect="4/3" radius="none" />,
  },
  Avatar: {
    initials: <Avatar name="Ada Lovelace" />,
    anonymous: <Avatar size="xl" shape="square" />,
    picture: <Avatar name="Ada Lovelace" src="/ada.jpg" size="lg" />,
  },
  Drawer: {
    closed: (
      <Drawer open={false} onClose={() => {}} title="Filters">
        Body
      </Drawer>
    ),
    open: (
      <Drawer
        open
        onClose={() => {}}
        title="Filters"
        description="Narrow the results"
        footer={<Button>Apply</Button>}
      >
        Body
      </Drawer>
    ),
    untitled: (
      <Drawer open onClose={() => {}} placement="left" size="sm">
        Body
      </Drawer>
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
