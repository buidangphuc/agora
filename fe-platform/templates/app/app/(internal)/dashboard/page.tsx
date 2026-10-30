import {
  Button,
  Checkbox,
  Dialog,
  Input,
  Select,
  Table,
} from "@fe-platform/ui";

const columns = [
  { key: "name", header: "Name" },
  { key: "status", header: "Status" },
];

const rows = [
  { id: 1, name: "Order 1001", status: "Shipped" },
  { id: 2, name: "Order 1002", status: "Pending" },
];

export default function Dashboard() {
  return (
    <main className="flex flex-col gap-6 p-6">
      <h1 className="text-2xl">Dashboard</h1>
      <p className="text-text-secondary">
        Internal route group. Authentication checks belong on this group's
        layout.
      </p>
      <form className="flex max-w-lg flex-col gap-4">
        <Input label="Name" helpText="As shown on invoices" />
        <Select
          label="Status"
          options={[
            { value: "open", label: "Open" },
            { value: "closed", label: "Closed" },
          ]}
        />
        <Checkbox label="Send me updates" />
        <div className="flex gap-2">
          <Button type="submit">Save</Button>
          <Dialog
            triggerLabel="Delete"
            title="Delete this record?"
            description="This cannot be undone."
          />
        </div>
      </form>
      <Table caption="Recent orders" columns={columns} rows={rows} />
    </main>
  );
}
