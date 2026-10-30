import type { ReactNode } from "react";
import {
  Button,
  Card,
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
  },
  Button: {
    default: <Button>Save</Button>,
    secondary: <Button variant="secondary">Cancel</Button>,
    ghost: <Button variant="ghost">More</Button>,
    disabled: <Button disabled>Save</Button>,
    loading: <Button loading>Saving</Button>,
  },
  Input: {
    default: <Input label="Name" />,
    help: <Input label="Name" helpText="As on your ID" />,
    error: <Input label="Name" errorText="Name is required" />,
    disabled: <Input label="Name" disabled />,
  },
  Checkbox: {
    default: <Checkbox label="Accept" />,
    checked: <Checkbox label="Accept" defaultChecked />,
    error: <Checkbox label="Accept" errorText="Required" />,
    disabled: <Checkbox label="Accept" disabled />,
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
