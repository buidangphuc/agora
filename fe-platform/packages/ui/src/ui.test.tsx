import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import axe from "axe-core";
import { useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import {
  Alert,
  Badge,
  Breadcrumb,
  Button,
  Card,
  CardContent,
  CardTitle,
  Checkbox,
  ClosableTag,
  Descriptions,
  Dialog,
  Empty,
  Input,
  Link,
  NativeSelect,
  PageSkeleton,
  Pagination,
  Progress,
  Result,
  Select,
  Skeleton,
  Statistic,
  Stepper,
  Table,
  Tabs,
  Tag,
  Timeline,
  ToastProvider,
  useToast,
} from "./index";
import { stateMatrix } from "./states";

afterEach(cleanup);

async function violations(node: Element) {
  const result = await axe.run(node, {
    rules: {
      "color-contrast": { enabled: false },
      region: { enabled: false },
    },
  });
  return result.violations.map((v) => `${v.id}: ${v.help}`);
}

describe("state matrix", () => {
  for (const [component, states] of Object.entries(stateMatrix)) {
    for (const [state, element] of Object.entries(states)) {
      it(`${component} ${state} has no axe violations`, async () => {
        const { baseElement } = render(element);
        expect(await violations(baseElement)).toEqual([]);
      });
    }
  }

  it("fails on a deliberately inaccessible fixture", async () => {
    const { baseElement } = render(<button type="button" />);
    expect((await violations(baseElement)).length).toBeGreaterThan(0);
  });
});

describe("Button", () => {
  it("defaults to type=button and marks loading as busy and disabled", () => {
    render(<Button loading>Saving</Button>);
    const button = screen.getByRole("button", { name: "Saving" });
    expect(button.getAttribute("type")).toBe("button");
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows a spinner only while loading and keeps the label", () => {
    const { container, rerender } = render(<Button loading>Saving</Button>);
    expect(container.querySelector("[aria-hidden='true']")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Saving" })).toBeTruthy();
    rerender(<Button>Saving</Button>);
    expect(container.querySelector("[aria-hidden='true']")).toBeNull();
  });

  it("renders icons around the label and keeps the accessible name", () => {
    render(
      <Button
        leftIcon={<span aria-hidden="true">L</span>}
        rightIcon={<span aria-hidden="true">R</span>}
      >
        Add
      </Button>,
    );
    expect(screen.getByRole("button", { name: "Add" }).textContent).toBe(
      "LAddR",
    );
  });

  it("applies danger, xs and fullWidth from the class maps", () => {
    render(
      <Button variant="danger" size="xs" fullWidth>
        Delete
      </Button>,
    );
    const cls = screen.getByRole("button", { name: "Delete" }).className;
    expect(cls).toContain("bg-action-danger");
    expect(cls).toContain("text-xs");
    expect(cls).toContain("w-full");
  });

  it("fullWidth mobile is full width below sm only", () => {
    render(<Button fullWidth="mobile">Save</Button>);
    const cls = screen.getByRole("button", { name: "Save" }).className;
    expect(cls).toContain("w-full");
    expect(cls).toContain("sm:w-auto");
  });

  it("forwards a ref", () => {
    let node: HTMLButtonElement | null = null;
    render(
      <Button
        ref={(el) => {
          node = el;
        }}
      >
        Go
      </Button>,
    );
    expect(node).toBeInstanceOf(HTMLButtonElement);
  });
});

describe("Card", () => {
  it("is padded by default and flush with padded={false}", () => {
    const { rerender } = render(<Card data-testid="c">x</Card>);
    expect(screen.getByTestId("c").className).toContain("p-6");
    rerender(
      <Card data-testid="c" padded={false}>
        x
      </Card>,
    );
    expect(screen.getByTestId("c").className).not.toContain("p-6");
  });

  it("replaces content with a busy skeleton while loading", () => {
    render(
      <Card data-testid="c" loading>
        secret
      </Card>,
    );
    const card = screen.getByTestId("c");
    expect(card.getAttribute("aria-busy")).toBe("true");
    expect(card.textContent).toBe("");
  });

  it("highlights on hover only when hoverable", () => {
    const { rerender } = render(<Card data-testid="c">x</Card>);
    expect(screen.getByTestId("c").className).not.toContain("hover:");
    rerender(
      <Card data-testid="c" hoverable>
        x
      </Card>,
    );
    expect(screen.getByTestId("c").className).toContain("hover:");
  });

  it("renders title as a heading inside content", () => {
    render(
      <Card padded={false}>
        <CardContent>
          <CardTitle>Desk lamp</CardTitle>
        </CardContent>
      </Card>,
    );
    expect(screen.getByRole("heading", { name: "Desk lamp" })).toBeTruthy();
  });
});

describe("Link", () => {
  it("is an underlined text link by default and keeps href, name and test id", () => {
    render(
      <Link href="/shop" data-testid="back">
        Back to shop
      </Link>,
    );
    const a = screen.getByRole("link", { name: "Back to shop" });
    expect(a.getAttribute("href")).toBe("/shop");
    expect(a.getAttribute("data-testid")).toBe("back");
    expect(a.className).toContain("underline");
  });

  it("subtle keeps the surrounding colour and drops the underline", () => {
    render(
      <Link href="/help" variant="subtle">
        Help
      </Link>,
    );
    const cls = screen.getByRole("link", { name: "Help" }).className;
    expect(cls).not.toContain("underline");
    expect(cls).toContain("hover:text-text-link");
  });

  it("draws a touch-sized button-shaped link from the button maps", () => {
    render(
      <Link href="/checkout" variant="primary" size="sm" fullWidth="mobile">
        Continue
      </Link>,
    );
    const cls = screen.getByRole("link", { name: "Continue" }).className;
    expect(cls).toContain("bg-action-primary");
    expect(cls).toContain("min-h-touch");
    expect(cls).toContain("text-sm");
    expect(cls).toContain("sm:w-auto");
  });
});

describe("Input", () => {
  it("announces the error through text linked by aria-describedby", () => {
    render(<Input label="Name" errorText="Name is required" />);
    const input = screen.getByLabelText("Name");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    const id = input.getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(id)?.textContent).toBe("Name is required");
  });

  it("marks a required field without changing its accessible name", () => {
    render(<Input label="Name" required />);
    const input = screen.getByRole("textbox", { name: "Name" });
    expect(input.hasAttribute("required")).toBe(true);
  });
});

describe("Checkbox", () => {
  it("toggles from the keyboard-accessible control", () => {
    let value = false;
    render(
      <Checkbox
        label="Accept"
        onCheckedChange={(v) => {
          value = v;
        }}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Accept" }));
    expect(value).toBe(true);
  });

  it("keeps a hidden label as the accessible name and reports the mixed state", () => {
    render(<Checkbox label="Select all" labelHidden checked="indeterminate" />);
    const box = screen.getByRole("checkbox", { name: "Select all" });
    expect(box.getAttribute("aria-checked")).toBe("mixed");
    expect(screen.getByText("Select all").className).toBe("sr-only");
  });
});

describe("Tag", () => {
  it("tints by tone and passes native attributes", () => {
    render(
      <Tag tone="success" data-testid="t">
        Paid
      </Tag>,
    );
    const tag = screen.getByTestId("t");
    expect(tag.className).toContain("text-text-success");
    expect(tag.textContent).toBe("Paid");
  });

  it("closes from a labelled touch-sized button and reports it", () => {
    let closed = 0;
    render(
      <ClosableTag closeLabel="Remove Price" onClose={() => closed++}>
        Price
      </ClosableTag>,
    );
    const close = screen.getByRole("button", { name: "Remove Price" });
    expect(close.className).toContain("min-h-touch");
    fireEvent.click(close);
    expect(closed).toBe(1);
    expect(screen.queryByText("Price")).toBeNull();
  });
});

describe("Badge", () => {
  it("switches between soft and solid fills and pill shape", () => {
    render(
      <>
        <Badge tone="danger" solid>
          Mall
        </Badge>
        <Badge pill>3</Badge>
      </>,
    );
    expect(screen.getByText("Mall").className).toContain("bg-action-danger");
    const pill = screen.getByText("3");
    expect(pill.className).toContain("rounded-full");
    expect(pill.className).toContain("text-text-link");
  });
});

describe("Empty", () => {
  it("shows a default illustration, the description and the action", () => {
    const { container } = render(
      <Empty description="Nothing here" action={<a href="/">Home</a>} />,
    );
    expect(container.querySelector("svg")).not.toBeNull();
    expect(screen.getByText("Nothing here")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Home" })).toBeTruthy();
  });

  it("replaces the illustration with a custom image", () => {
    const { container } = render(
      <Empty description="x" image={<span>img</span>} />,
    );
    expect(container.querySelector("svg")).toBeNull();
  });
});

describe("Result", () => {
  it("announces politely with a named status icon and heading", () => {
    const { container } = render(
      <Result status="error" title="Payment failed" statusLabel="Lỗi" />,
    );
    expect(
      (container.firstChild as HTMLElement).getAttribute("aria-live"),
    ).toBe("polite");
    expect(screen.getByRole("img", { name: "Lỗi" })).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Payment failed" }),
    ).toBeTruthy();
  });

  it("falls back to an English status label", () => {
    render(<Result status="404" title="Missing" />);
    expect(screen.getByRole("img", { name: "Not found" })).toBeTruthy();
  });
});

describe("Select", () => {
  it("opens from the keyboard and selects an option", async () => {
    let value = "";
    render(
      <Select
        label="Letter"
        options={[
          { value: "a", label: "Alpha" },
          { value: "b", label: "Beta" },
        ]}
        onValueChange={(v) => {
          value = v;
        }}
      />,
    );
    const trigger = screen.getByRole("combobox", { name: "Letter" });
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter" });
    const option = await screen.findByRole("option", { name: "Beta" });
    expect(await violations(document.body)).toEqual([]);
    fireEvent.click(option);
    expect(value).toBe("b");
  });
});

describe("NativeSelect", () => {
  const options = [
    { value: "a", label: "Alpha" },
    { value: "b", label: "Beta", disabled: true },
  ];

  it("renders a labelled native select that reports changes", () => {
    let value = "";
    render(
      <NativeSelect
        label="Letter"
        name="letter"
        data-testid="letter"
        placeholder="Pick"
        options={options}
        onChange={(e) => {
          value = e.target.value;
        }}
      />,
    );
    const box = screen.getByRole("combobox", { name: "Letter" });
    expect(box.tagName).toBe("SELECT");
    expect(box.getAttribute("name")).toBe("letter");
    expect(screen.getByTestId("letter")).toBe(box);
    expect((box as HTMLSelectElement).value).toBe("");
    expect(
      (screen.getByRole("option", { name: "Pick" }) as HTMLOptionElement)
        .disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("option", { name: "Beta" }) as HTMLOptionElement)
        .disabled,
    ).toBe(true);
    fireEvent.change(box, { target: { value: "a" } });
    expect(value).toBe("a");
  });

  it("links help and error text and marks the field invalid", () => {
    render(
      <NativeSelect
        label="Letter"
        options={options}
        helpText="Any letter"
        errorText="Pick one"
        required
      />,
    );
    const box = screen.getByRole("combobox", { name: "Letter" });
    expect(box.getAttribute("aria-invalid")).toBe("true");
    const ids = box.getAttribute("aria-describedby")?.split(" ") ?? [];
    expect(ids.map((id) => document.getElementById(id)?.textContent)).toEqual([
      "Pick one",
      "Any letter",
    ]);
    expect((box as HTMLSelectElement).required).toBe(true);
  });

  it("keeps the accessible name when the label is hidden", () => {
    render(<NativeSelect label="Letter" labelHidden options={options} />);
    expect(screen.getByRole("combobox", { name: "Letter" })).toBeTruthy();
    expect(screen.getByText("Letter").className).toBe("sr-only");
  });
});

describe("Dialog", () => {
  it("opens, exposes title and description, and closes on Escape", () => {
    render(
      <Dialog
        triggerLabel="Open"
        title="Confirm"
        description="Are you sure?"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    const dialog = screen.getByRole("dialog", { name: "Confirm" });
    expect(dialog.getAttribute("aria-describedby")).toBeTruthy();
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("runs controlled without a trigger, shows footer actions and returns focus to the opener", async () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Opener
          </button>
          <Dialog
            open={open}
            onOpenChange={setOpen}
            title="Remove item"
            closeLabel="Dismiss"
            size="sm"
            footer={<button type="button">Remove</button>}
          >
            body
          </Dialog>
        </>
      );
    }
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Opener" });
    opener.focus();
    fireEvent.click(opener);
    const dialog = screen.getByRole("dialog", { name: "Remove item" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.getAttribute("aria-describedby")).toBeNull();
    expect(
      dialog.contains(screen.getByRole("button", { name: "Remove" })),
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});

describe("Skeleton", () => {
  it("is busy, labelled for screen readers and renders the requested lines", () => {
    const { container } = render(<Skeleton lines={4} label="Wait" />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.getAttribute("aria-busy")).toBe("true");
    expect(root.getAttribute("data-variant")).toBe("text");
    expect(screen.getByText("Wait").className).toBe("sr-only");
    expect(container.querySelectorAll(".h-4")).toHaveLength(4);
  });

  it("reserves the aspect box of image and card presets", () => {
    const { container, rerender } = render(
      <Skeleton variant="image" aspect="4/3" />,
    );
    expect(container.querySelector(".aspect-4\\/3")).not.toBeNull();
    rerender(<Skeleton variant="avatar" size="sm" />);
    expect(container.querySelector(".rounded-full.h-8")).not.toBeNull();
  });

  it("page skeleton is a busy live region with a tile grid", () => {
    render(<PageSkeleton data-testid="page" />);
    const page = screen.getByTestId("page");
    expect(page.getAttribute("aria-busy")).toBe("true");
    expect(page.getAttribute("aria-live")).toBe("polite");
    expect(page.querySelectorAll(".animate-pulse").length).toBeGreaterThan(5);
  });
});

describe("Progress", () => {
  it("exposes a clamped value on a named progressbar", () => {
    const { container, rerender } = render(
      <Progress label="Upload" percent={40} />,
    );
    const bar = screen.getByRole("progressbar", { name: "Upload" });
    expect(bar.getAttribute("aria-valuenow")).toBe("40");
    expect(
      (container.querySelector("[style]") as HTMLElement).style.width,
    ).toBe("40%");
    expect(screen.getByText("40%")).toBeTruthy();
    rerender(<Progress label="Upload" percent={150} showInfo={false} />);
    expect(bar.getAttribute("aria-valuenow")).toBe("100");
    expect(screen.queryByText("100%")).toBeNull();
  });

  it("colours the fill by status", () => {
    const { container } = render(
      <Progress label="Upload" percent={10} status="danger" />,
    );
    expect(container.querySelector(".bg-action-danger")).not.toBeNull();
  });
});

describe("Descriptions", () => {
  const items = [
    { key: "a", label: "Name", children: "Lamp" },
    { key: "b", label: "Qty", children: "2" },
  ];

  it("renders pairs as a description list under a heading", () => {
    const { container } = render(<Descriptions title="Order" items={items} />);
    expect(screen.getByRole("heading", { name: "Order" })).toBeTruthy();
    expect(container.querySelectorAll("dt")).toHaveLength(2);
    expect(container.querySelector("dd")?.textContent).toBe("Lamp");
  });

  it("shows busy skeleton rows, the empty text or the fallback instead of values", () => {
    const { container, rerender } = render(
      <Descriptions items={items} loading />,
    );
    expect(screen.queryByText("Lamp")).toBeNull();
    expect(container.querySelectorAll('[data-variant="text"]')).toHaveLength(2);
    rerender(<Descriptions items={[]} emptyText="None" />);
    expect(screen.getByText("None")).toBeTruthy();
    rerender(<Descriptions items={items} fallback={<p>Broken</p>} />);
    expect(screen.getByText("Broken")).toBeTruthy();
    expect(container.querySelector("dl")).toBeNull();
  });
});

describe("Statistic", () => {
  it("renders the value with prefix, suffix and an upward trend", () => {
    render(
      <Statistic
        title="Revenue"
        value="1,200"
        prefix="$"
        suffix="k"
        trend={{ value: "5%", isUp: true, label: "vs last week" }}
      />,
    );
    expect(screen.getByText("1,200")).toBeTruthy();
    expect(screen.getByText("↑").parentElement?.className).toContain(
      "text-text-success",
    );
    expect(screen.getByText("vs last week")).toBeTruthy();
  });

  it("keeps the fixed-height rows while loading and marks itself busy", () => {
    const trend = { value: "5%" };
    const rows = (root: Element) =>
      Array.from(root.querySelectorAll(".h-8, .mt-3.h-4")).map(
        (el) => el.className,
      );
    const loaded = render(
      <Statistic title="Orders" value="12" trend={trend} />,
    );
    const loadedRows = rows(loaded.container);
    loaded.unmount();
    const { container } = render(
      <Statistic title="Orders" value="12" trend={trend} loading />,
    );
    expect(screen.queryByText("12")).toBeNull();
    expect(container.firstElementChild?.getAttribute("aria-busy")).toBe("true");
    expect(rows(container)).toEqual(loadedRows);
    expect(loadedRows).toHaveLength(2);
  });

  it("replaces the value with the fallback and colours a danger tone", () => {
    const { rerender } = render(
      <Statistic title="Orders" value="12" fallback={<p>Broken</p>} />,
    );
    expect(screen.getByText("Broken")).toBeTruthy();
    expect(screen.queryByText("12")).toBeNull();
    rerender(<Statistic title="Overdue" value="3" tone="danger" />);
    expect(screen.getByText("3").parentElement?.className).toContain(
      "text-text-danger",
    );
  });
});

function Fire() {
  const { toast } = useToast();
  return (
    <button type="button" onClick={() => toast({ title: "Saved" })}>
      Go
    </button>
  );
}

describe("Toast", () => {
  it("shows a message in a live region and can be dismissed", async () => {
    render(
      <ToastProvider>
        <Fire />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    await screen.findByText("Saved", { selector: "li *" });
    await waitFor(() => {
      const live = screen.getAllByRole("status");
      expect(live.some((el) => el.textContent?.includes("Saved"))).toBe(true);
    });
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Saved")).toBeNull();
  });
});

describe("Table", () => {
  type Row = { sku: string; name: string; qty: number };
  const rows: Row[] = [
    { sku: "a", name: "Pen", qty: 2 },
    { sku: "b", name: "Ink", qty: 5 },
  ];
  const columns = [
    { key: "name", header: "Name" },
    {
      key: "qty",
      header: "Qty",
      align: "right" as const,
      render: (r: Row) => `x${r.qty}`,
    },
  ];

  it("renders custom cells, row test ids and a hidden caption as the name", () => {
    render(
      <Table<Row>
        caption="Items"
        captionHidden
        columns={columns}
        rows={rows}
        rowKey={(r) => r.sku}
        rowTestId={(r) => `row-${r.sku}`}
      />,
    );
    expect(screen.getByRole("table", { name: "Items" })).toBeTruthy();
    expect(screen.getByTestId("row-b").textContent).toBe("Inkx5");
    expect(screen.getAllByRole("columnheader")).toHaveLength(2);
  });

  it("keeps the headers and marks the table busy while loading", () => {
    render(
      <Table<Row>
        caption="Items"
        columns={columns}
        rows={rows}
        loading
        loadingRows={3}
      />,
    );
    expect(screen.getByRole("table").getAttribute("aria-busy")).toBe("true");
    expect(screen.getAllByRole("row")).toHaveLength(4);
    expect(screen.queryByText("Pen")).toBeNull();
  });

  it("shows an error alert with a retry action instead of rows", () => {
    let retried = 0;
    render(
      <Table<Row>
        caption="Items"
        columns={columns}
        rows={rows}
        error="Could not load"
        onRetry={() => {
          retried += 1;
        }}
        retryLabel="Retry"
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("Could not load");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retried).toBe(1);
    expect(screen.queryByText("Pen")).toBeNull();
  });
});

function FireTone({ tone }: { tone: "success" | "danger" }) {
  const { toast } = useToast();
  return (
    <button
      type="button"
      onClick={() => toast({ title: `${tone} message`, tone, duration: 0 })}
    >
      Go
    </button>
  );
}

describe("Toast tones", () => {
  it("announces a danger toast as an alert", async () => {
    render(
      <ToastProvider dismissLabel="Close">
        <FireTone tone="danger" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("danger message");
    expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
  });

  it("keeps other toasts as status and out of the alert role", async () => {
    render(
      <ToastProvider>
        <FireTone tone="success" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    await screen.findByText("success message", { selector: "li *" });
    const matches = screen
      .getAllByRole("status")
      .filter((el) => el.textContent?.includes("success message"));
    expect(matches).toHaveLength(1);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("Alert", () => {
  it("uses role=alert for danger and role=status for the other tones", () => {
    const { rerender } = render(<Alert tone="danger" title="Failed" />);
    expect(screen.getByRole("alert").textContent).toContain("Failed");
    for (const tone of ["info", "success", "warning"] as const) {
      rerender(<Alert tone={tone} title={tone} />);
      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.getByRole("status").textContent).toContain(tone);
    }
  });

  it("renders description and action and has no close button by default", () => {
    render(
      <Alert
        tone="warning"
        title="Low stock"
        description="2 left"
        action={<a href="/x">View</a>}
      />,
    );
    expect(screen.getByText("2 left")).toBeTruthy();
    expect(screen.getByRole("link", { name: "View" })).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("closes itself and reports it when closable", () => {
    let closed = 0;
    render(
      <Alert
        tone="success"
        title="Saved"
        closable
        closeLabel="Close"
        onClose={() => {
          closed += 1;
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(closed).toBe(1);
    expect(screen.queryByText("Saved")).toBeNull();
describe("Tabs", () => {
  const items = [
    { id: "all", label: "All", content: <p>All panel</p> },
    { id: "open", label: "Open", badge: 3, content: <p>Open panel</p> },
    { id: "off", label: "Off", disabled: true },
    { id: "done", label: "Done", content: <p>Done panel</p> },
  ];

  it("wires tablist, tabs and panels and moves with arrow keys past disabled tabs", () => {
    const seen: string[] = [];
    render(<Tabs items={items} onChange={(id) => seen.push(id)} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs[0].getAttribute("aria-selected")).toBe("true");
    const panel = screen.getByRole("tabpanel", { name: "All" });
    expect(tabs[0].getAttribute("aria-controls")).toBe(panel.id);
    fireEvent.keyDown(tabs[1], { key: "ArrowRight" });
    expect(seen).toEqual(["done"]);
    expect(document.activeElement).toBe(tabs[3]);
    expect(screen.getByRole("tabpanel").textContent).toBe("Done panel");
    fireEvent.keyDown(tabs[3], { key: "Home" });
    expect(tabs[0].getAttribute("tabindex")).toBe("0");
    expect(tabs[3].getAttribute("tabindex")).toBe("-1");
  });

  it("keeps every tab at the touch height with a keyboard focus ring", () => {
    render(<Tabs items={items} variant="pills" />);
    for (const tab of screen.getAllByRole("tab")) {
      expect(tab.className).toContain("min-h-touch");
      expect(tab.className).toContain("focus-visible:ring-2");
    }
  });

  it("renders server links with aria-current when hrefFor is given", () => {
    const html = renderToStaticMarkup(
      <Tabs items={items} activeId="open" hrefFor={(id) => `/o/${id}`} />,
    );
    expect(html).toContain('href="/o/open"');
    expect(html).toContain('aria-current="page"');
    expect(html).toContain('aria-label="Tabs"');
    expect(html).not.toContain("<button");
  });
});

describe("Pagination", () => {
  const hrefFor = (p: number) => `/s?page=${p}`;

  it("collapses long ranges around the current page with custom labels", () => {
    render(
      <Pagination
        current={10}
        total={200}
        hrefFor={hrefFor}
        pageLabel={(p) => `P${p}`}
        previousLabel="Prev"
        nextLabel="Next"
      />,
    );
    expect(
      screen.getAllByRole("link").map((l) => l.getAttribute("aria-label")),
    ).toEqual(["Prev", "P1", "P9", "P10", "P11", "P20", "Next"]);
    expect(
      screen.getByRole("link", { name: "P10" }).getAttribute("aria-current"),
    ).toBe("page");
  });

  it("disables the bound arrows, clamps the page and hides a single page", () => {
    const { container, rerender } = render(
      <Pagination current={99} total={30} hrefFor={hrefFor} />,
    );
    expect(
      screen.getByLabelText("Next page").getAttribute("aria-disabled"),
    ).toBe("true");
    expect(
      screen.getByRole("link", { name: "Page 3" }).getAttribute("aria-current"),
    ).toBe("page");
    rerender(<Pagination current={1} total={5} hrefFor={hrefFor} />);
    expect(container.innerHTML).toBe("");
  });
});

describe("Breadcrumb", () => {
  it("links ancestors and marks the last item as the current page", () => {
    render(
      <Breadcrumb
        items={[
          { label: "Home", href: "/" },
          { label: "Pixel", href: "/p" },
        ]}
      />,
    );
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toBeTruthy();
    expect(
      screen.getAllByRole("link").map((l) => l.getAttribute("href")),
    ).toEqual(["/"]);
    const current = screen.getByText("Pixel");
    expect(current.getAttribute("aria-current")).toBe("page");
    expect(current.closest("a")).toBeNull();
  });
});

describe("Stepper", () => {
  it("marks the current step and links only enabled steps with an href", () => {
    render(
      <Stepper
        steps={[
          { id: 1, title: "Address", status: "complete", href: "/a" },
          {
            id: 2,
            title: "Shipping",
            status: "complete",
            href: "/b",
            disabled: true,
          },
          { id: 3, title: "Pay", status: "current" },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items[2].getAttribute("aria-current")).toBe("step");
    expect(items[0].getAttribute("aria-current")).toBeNull();
    expect(
      screen.getByRole("link", { name: "Address" }).getAttribute("href"),
    ).toBe("/a");
    expect(screen.queryByRole("link", { name: "Shipping" })).toBeNull();
    expect(screen.getByText("Shipping").getAttribute("aria-disabled")).toBe(
      "true",
    );
  });

  it("lists descriptions in the vertical orientation", () => {
    render(
      <Stepper
        orientation="vertical"
        steps={[
          { id: 1, title: "Paid", status: "complete" },
          { id: 2, title: "Failed", status: "failed", description: "Retry" },
        ]}
      />,
    );
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(screen.getByText("Retry")).toBeTruthy();
  });
});

describe("Timeline", () => {
  it("renders events in order with time, description, testId and the current mark", () => {
    render(
      <Timeline
        items={[
          { key: "1", title: "Ordered", time: "10:00", testId: "evt" },
          {
            key: "2",
            title: "Shipping",
            description: "With carrier",
            tone: "primary",
            current: true,
          },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[1].getAttribute("aria-current")).toBe("step");
    expect(screen.getByTestId("evt").tagName).toBe("LI");
    expect(screen.getByText("10:00")).toBeTruthy();
    expect(screen.getByText("With carrier")).toBeTruthy();
  });
});

const CLIENT_ONLY =
  /@radix-ui\/|\buse(?:State|Effect|Ref|Context|Callback|Memo|Reducer)\b|\bcreateContext\b/;

function clientBoundaryViolations(files: Record<string, string>) {
  return Object.entries(files)
    .filter(([, text]) => CLIENT_ONLY.test(text))
    .filter(([, text]) => !/^["']use client["'];?/.test(text.trimStart()))
    .map(([name]) => name);
}

describe("client boundary", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const files = Object.fromEntries(
    readdirSync(here)
      .filter((f) => /\.tsx?$/.test(f) && !/\.test\.|states\./.test(f))
      .map((f) => [f, readFileSync(join(here, f), "utf8")]),
  );

  it("marks every client-only file with use client", () => {
    expect(clientBoundaryViolations(files)).toEqual([]);
  });

  it("keeps Button, Input, Table and the status and data display components server-safe", () => {
    for (const name of [
      "Button.tsx",
      "Input.tsx",
      "Table.tsx",
      "Tag.tsx",
      "Badge.tsx",
      "Empty.tsx",
      "Result.tsx",
      "Skeleton.tsx",
      "Progress.tsx",
      "Descriptions.tsx",
      "Statistic.tsx",
    ]) {
      expect(files[name]).not.toMatch(/^["']use client["']/);
    }
  });

  it("detects a seeded violation", () => {
    const seeded = {
      "Bad.tsx": 'import * as R from "@radix-ui/react-dialog";\nexport {};',
    };
    expect(clientBoundaryViolations(seeded)).toEqual(["Bad.tsx"]);
  });
});
