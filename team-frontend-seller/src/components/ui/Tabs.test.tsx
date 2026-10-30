import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Tabs } from "./Tabs";

const items = [
  { id: "all", label: "Tất cả", content: <p>Panel tất cả</p> },
  { id: "shipping", label: "Đang giao", badge: 3, content: <p>Panel giao</p> },
  { id: "done", label: "Hoàn tất", content: <p>Panel xong</p> },
];

describe("Tabs (client)", () => {
  it("has tablist, tab and tabpanel roles wired together", () => {
    render(<Tabs items={items} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(3);
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    expect(tabs[1]).toHaveAttribute("aria-selected", "false");
    const panel = screen.getByRole("tabpanel");
    expect(panel).toHaveAccessibleName("Tất cả");
    expect(tabs[0]).toHaveAttribute("aria-controls", panel.id);
  });

  it("ArrowRight selects the next tab and shows its panel", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Tabs items={items} onChange={onChange} />);
    const tabs = screen.getAllByRole("tab");
    tabs[0].focus();
    await user.keyboard("{ArrowRight}");
    expect(tabs[1]).toHaveAttribute("aria-selected", "true");
    expect(tabs[1]).toHaveFocus();
    expect(onChange).toHaveBeenCalledWith("shipping");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Panel giao");
  });

  it("ArrowLeft wraps, Home and End jump, tabindex is roving", async () => {
    const user = setupUser();
    render(<Tabs items={items} />);
    const tabs = screen.getAllByRole("tab");
    tabs[0].focus();
    await user.keyboard("{ArrowLeft}");
    expect(tabs[2]).toHaveAttribute("aria-selected", "true");
    expect(tabs[2]).toHaveAttribute("tabindex", "0");
    expect(tabs[0]).toHaveAttribute("tabindex", "-1");
    await user.keyboard("{Home}");
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{End}");
    expect(tabs[2]).toHaveAttribute("aria-selected", "true");
  });

  it("skips disabled tabs when arrowing", async () => {
    const user = setupUser();
    render(
      <Tabs
        items={[
          { id: "a", label: "A" },
          { id: "b", label: "B", disabled: true },
          { id: "c", label: "C" },
        ]}
      />,
    );
    screen.getByRole("tab", { name: "A" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "C" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("controlled activeId wins and click selects", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <Tabs
        items={items}
        activeId="done"
        onChange={onChange}
        variant="pills"
      />,
    );
    expect(screen.getByRole("tab", { name: "Hoàn tất" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.click(screen.getByRole("tab", { name: /Đang giao/ }));
    expect(onChange).toHaveBeenCalledWith("shipping");
  });

  it("renders the badge and keeps a keyboard-only focus ring", () => {
    render(<Tabs items={items} />);
    expect(screen.getByRole("tab", { name: /Đang giao/ })).toHaveTextContent(
      "3",
    );
    expect(screen.getAllByRole("tab")[0].className).toContain(
      "focus-visible:ring-2",
    );
  });
});

describe("Tabs (link variant)", () => {
  const hrefFor = (id: string) => `/account/orders?status=${id}`;

  it("renders links with aria-current on the active tab", () => {
    render(<Tabs items={items} activeId="shipping" hrefFor={hrefFor} />);
    const nav = screen.getByRole("navigation", { name: "Tabs" });
    expect(nav).toBeInTheDocument();
    const links = screen.getAllByRole("link");
    expect(links.map((l) => l.getAttribute("href"))).toEqual([
      "/account/orders?status=all",
      "/account/orders?status=shipping",
      "/account/orders?status=done",
    ]);
    expect(links[1]).toHaveAttribute("aria-current", "page");
    expect(links[0]).not.toHaveAttribute("aria-current");
    expect(screen.queryByRole("tablist")).toBeNull();
  });

  it("server-renders without client state or button elements", () => {
    const html = renderToStaticMarkup(
      <Tabs items={items} activeId="shipping" hrefFor={hrefFor} />,
    );
    expect(html).toContain('href="/account/orders?status=shipping"');
    expect(html).toContain('aria-current="page"');
    expect(html).not.toContain("<button");
  });
});
