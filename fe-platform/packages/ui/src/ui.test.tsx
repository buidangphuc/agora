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
import { afterEach, describe, expect, it } from "vitest";
import {
  Button,
  Card,
  CardContent,
  CardTitle,
  Checkbox,
  Descriptions,
  Dialog,
  Input,
  PageSkeleton,
  Progress,
  Select,
  Skeleton,
  Statistic,
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

  it("keeps Button, Input, Table and the data display components server-safe", () => {
    for (const name of [
      "Button.tsx",
      "Input.tsx",
      "Table.tsx",
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
