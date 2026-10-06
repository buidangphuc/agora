import { render, screen, within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Table, type TableColumn } from "./Table";

interface Row {
  id: string;
  name: string;
  qty: number;
}
const columns: TableColumn<Row>[] = [
  { key: "name", title: "Tên", dataIndex: "name" },
  { key: "qty", title: "SL", dataIndex: "qty", align: "right" },
  {
    key: "act",
    title: "Thao tác",
    render: (r) => <a href={`/p/${r.id}`}>Sửa {r.name}</a>,
  },
];
const data: Row[] = [
  { id: "a", name: "Áo", qty: 2 },
  { id: "b", name: "Quần", qty: 5 },
];

describe("Table", () => {
  it("renders headers, rows, field values and custom cells", () => {
    render(
      <Table
        columns={columns}
        dataSource={data}
        rowKey="id"
        caption="Sản phẩm"
      />,
    );
    expect(screen.getByRole("table", { name: "Sản phẩm" })).toBeInTheDocument();
    expect(
      screen.getAllByRole("columnheader").map((h) => h.textContent),
    ).toEqual(["Tên", "SL", "Thao tác"]);
    expect(screen.getAllByRole("row")).toHaveLength(3);
    expect(screen.getByRole("link", { name: "Sửa Quần" })).toHaveAttribute(
      "href",
      "/p/b",
    );
  });

  it("an empty table shows the headers and an Empty block, not a blank area", () => {
    render(
      <Table
        columns={columns}
        dataSource={[]}
        rowKey="id"
        emptyText="Chưa có sản phẩm"
      />,
    );
    expect(screen.getAllByRole("columnheader")).toHaveLength(3);
    expect(screen.getByText("Chưa có sản phẩm")).toBeInTheDocument();
  });

  it("falls back to the default Empty text", () => {
    render(<Table columns={columns} dataSource={[]} rowKey="id" />);
    expect(screen.getByText("Không có dữ liệu")).toBeInTheDocument();
  });

  it("loading renders skeleton rows with the same headers and marks the table busy", () => {
    render(
      <Table
        columns={columns}
        dataSource={data}
        rowKey="id"
        loading
        loadingRows={3}
      />,
    );
    expect(screen.getByRole("table")).toHaveAttribute("aria-busy", "true");
    expect(screen.getAllByRole("columnheader")).toHaveLength(3);
    expect(screen.queryByText("Áo")).toBeNull();
    // header + 3 skeleton rows
    expect(screen.getAllByRole("row")).toHaveLength(4);
  });

  it("error shows an inline alert with a working retry", async () => {
    const user = setupUser();
    const onRetry = vi.fn();
    render(
      <Table
        columns={columns}
        dataSource={data}
        rowKey="id"
        error="Không tải được"
        onRetry={onRetry}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Không tải được");
    await user.click(within(alert).getByRole("button", { name: "Thử lại" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("rowKey may be a function and the table is server-renderable", () => {
    const html = renderToStaticMarkup(
      <Table
        columns={columns}
        dataSource={data}
        rowKey={(r) => `row-${r.id}`}
      />,
    );
    expect(html).toContain("Quần");
  });
});
