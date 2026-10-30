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
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import {
  Breadcrumb,
  Button,
  Card,
  CardContent,
  CardTitle,
  Checkbox,
  Dialog,
  Input,
  Pagination,
  Select,
  Stepper,
  Tabs,
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

  it("keeps Button, Input and Table server-safe", () => {
    for (const name of ["Button.tsx", "Input.tsx", "Table.tsx"]) {
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
