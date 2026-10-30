import { Button, Link, Table } from "@fe-platform/ui";

export default function Home() {
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Consumer example</h1>
      <section className="rounded-xl border border-border-subtle bg-surface-card p-4">
        <p className="text-text-secondary">
          Installed from the registry, not linked from the workspace.
        </p>
        <Button>Primary action</Button>
      </section>
      <Table
        caption="Packages"
        columns={[
          { key: "name", header: "Package" },
          { key: "role", header: "Role" },
        ]}
        rows={[
          { id: 1, name: "tokens", role: "Design tokens" },
          { id: 2, name: "ui", role: "Components" },
        ]}
      />
      <Link href="/">Home</Link>
    </main>
  );
}
