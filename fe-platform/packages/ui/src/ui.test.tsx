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
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  Avatar,
  Button,
  Card,
  CardContent,
  CardTitle,
  Checkbox,
  Dialog,
  Drawer,
  Image,
  Input,
  PriceTag,
  Select,
  ToastProvider,
  formatPrice,
  initials,
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

describe("PriceTag", () => {
  it("formats dong with a leading symbol", () => {
    expect(formatPrice(29990000)).toBe("₫29.990.000");
  });

  it("shows the struck original price and the rounded discount", () => {
    render(<PriceTag price={75000} originalPrice={100000} />);
    expect(screen.getByText("₫100.000").className).toContain("line-through");
    expect(screen.getByText("-25%")).toBeTruthy();
  });

  it("hides the discount when the original is not higher", () => {
    render(<PriceTag price={100000} originalPrice={100000} />);
    expect(screen.queryByText(/^-\d+%$/)).toBeNull();
  });
});

describe("Image", () => {
  it("reserves the aspect box, lazy-loads and falls back on error", () => {
    const { container } = render(
      <Image src="/a.jpg" alt="Lamp" aspect="square" />,
    );
    const box = container.firstElementChild as HTMLElement;
    expect(box.className).toContain("aspect-square");
    const img = screen.getByAltText("Lamp");
    expect(img.getAttribute("loading")).toBe("lazy");
    fireEvent.error(img);
    expect(screen.queryByAltText("Lamp")).toBeNull();
    expect(screen.getByRole("img", { name: "No image" })).toBeTruthy();
    expect(box.className).toContain("aspect-square");
  });

  it("renders the fallback in the server markup for an empty src", () => {
    const html = renderToStaticMarkup(
      <Image src="" alt="Lamp" aspect="2/1" fallbackLabel="Không có ảnh" />,
    );
    expect(html).not.toContain("<img");
    expect(html).toContain("Không có ảnh");
    expect(html).toContain("aspect-2/1");
  });

  it("drops the placeholder once loaded", () => {
    const { container } = render(
      <Image src="/a.jpg" alt="Lamp" aspect="4/3" radius="none" />,
    );
    expect(container.querySelector(".animate-pulse")).not.toBeNull();
    fireEvent.load(screen.getByAltText("Lamp"));
    expect(container.querySelector(".animate-pulse")).toBeNull();
  });
});

describe("Avatar", () => {
  it("derives up to two initials", () => {
    expect(initials("Nguyễn Văn A")).toBe("NA");
    expect(initials("")).toBe("?");
  });

  it("is a named image with initials when there is no src", () => {
    render(<Avatar name="Ada Lovelace" />);
    expect(screen.getByRole("img", { name: "Ada Lovelace" }).textContent).toBe(
      "AL",
    );
  });

  it("uses the anonymous label without a name", () => {
    render(<Avatar anonymousLabel="Người dùng" />);
    expect(screen.getByRole("img", { name: "Người dùng" })).toBeTruthy();
  });

  it("falls back to initials when the picture fails", () => {
    render(<Avatar name="Ada Lovelace" src="/ada.jpg" />);
    fireEvent.error(screen.getByAltText("Ada Lovelace"));
    expect(screen.getByText("AL")).toBeTruthy();
  });
});

describe("Drawer", () => {
  it("is a labelled modal side panel that closes on Escape and the close button", () => {
    const onClose = vi.fn();
    render(
      <Drawer
        open
        onClose={onClose}
        title="Filters"
        description="Narrow the results"
        placement="left"
      >
        Body
      </Drawer>,
    );
    const panel = screen.getByRole("dialog", { name: "Filters" });
    expect(panel.getAttribute("aria-modal")).toBe("true");
    expect(panel.className).toContain("left-0");
    const describedBy = panel.getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(describedBy)?.textContent).toBe(
      "Narrow the results",
    );
    fireEvent.keyDown(panel, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("returns focus to the element that opened it", async () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Filters
          </button>
          <Drawer open={open} onClose={() => setOpen(false)} title="Filters">
            Body
          </Drawer>
        </>
      );
    }
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Filters" });
    opener.focus();
    fireEvent.click(opener);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("names an untitled panel from its label and renders nothing while closed", () => {
    const { rerender } = render(
      <Drawer open onClose={() => {}} label="Menu">
        Body
      </Drawer>,
    );
    expect(screen.getByRole("dialog", { name: "Menu" })).toBeTruthy();
    rerender(
      <Drawer open={false} onClose={() => {}} label="Menu">
        Body
      </Drawer>,
    );
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
