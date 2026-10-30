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
  Alert,
  Button,
  Card,
  CardContent,
  CardTitle,
  Checkbox,
  Dialog,
  Input,
  Select,
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
