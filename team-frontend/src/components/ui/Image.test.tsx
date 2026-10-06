import { fireEvent, render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { Image } from "./Image";

describe("Image", () => {
  it("lazy-loads by default and reserves its aspect box", () => {
    const { container } = render(
      <Image src="/a.jpg" alt="Áo" aspect="square" />,
    );
    expect(screen.getByAltText("Áo")).toHaveAttribute("loading", "lazy");
    expect(container.firstChild).toHaveClass("aspect-square");
  });

  it("allows eager loading", () => {
    render(<Image src="/a.jpg" alt="Áo" aspect="video" loading="eager" />);
    expect(screen.getByAltText("Áo")).toHaveAttribute("loading", "eager");
  });

  it("the box is square before and after a failed load and the fallback shows", () => {
    const { container } = render(
      <Image src="/broken.jpg" alt="Áo" aspect="square" />,
    );
    const box = container.firstChild as HTMLElement;
    expect(box).toHaveClass("aspect-square");
    fireEvent.error(screen.getByAltText("Áo"));
    expect(screen.queryByAltText("Áo")).toBeNull();
    expect(
      screen.getByRole("img", { name: "Không có ảnh" }),
    ).toBeInTheDocument();
    expect(box).toHaveClass("aspect-square");
  });

  it("renders the fallback in the first paint for an empty src (no broken img swap)", () => {
    const html = renderToStaticMarkup(
      <Image src="" alt="Áo" aspect="square" />,
    );
    expect(html).not.toContain("<img");
    expect(html).toContain("Không có ảnh");
  });

  it("shows a pulse placeholder until the picture loads", () => {
    const { container } = render(<Image src="/a.jpg" alt="Áo" aspect="4/3" />);
    expect(container.querySelector(".animate-pulse")).not.toBeNull();
    fireEvent.load(screen.getByAltText("Áo"));
    expect(container.querySelector(".animate-pulse")).toBeNull();
  });

  it("supports a custom fallback", () => {
    render(
      <Image
        src="/x.jpg"
        alt="x"
        aspect="3/4"
        fallback={<span>Ảnh hỏng</span>}
      />,
    );
    fireEvent.error(screen.getByAltText("x"));
    expect(screen.getByText("Ảnh hỏng")).toBeInTheDocument();
  });

  it("server-renders the aspect box before hydration", () => {
    const html = renderToStaticMarkup(
      <Image src="/a.jpg" alt="a" aspect="2/1" />,
    );
    expect(html).toContain("aspect-2/1");
    expect(html).toContain('loading="lazy"');
  });

  it("a new src resets the load state", () => {
    const { rerender } = render(<Image src="/a.jpg" alt="x" aspect="square" />);
    fireEvent.error(screen.getByAltText("x"));
    rerender(<Image src="/b.jpg" alt="x" aspect="square" />);
    expect(screen.getByAltText("x")).toHaveAttribute("src", "/b.jpg");
  });

  describe("pictures that settled before hydration", () => {
    const proto = HTMLImageElement.prototype;
    const restore: Array<() => void> = [];
    const stub = (complete: boolean, naturalWidth: number) => {
      for (const [key, value] of [
        ["complete", complete],
        ["naturalWidth", naturalWidth],
      ] as const) {
        Object.defineProperty(proto, key, {
          configurable: true,
          get: () => value,
        });
        restore.push(() => Reflect.deleteProperty(proto, key));
      }
    };
    afterEach(() => {
      for (const undo of restore.splice(0)) undo();
    });

    it("an already-failed picture shows the fallback without waiting for onError", () => {
      stub(true, 0);
      render(<Image src="/broken.jpg" alt="x" aspect="square" />);
      expect(screen.queryByAltText("x")).toBeNull();
      expect(
        screen.getByRole("img", { name: "Không có ảnh" }),
      ).toBeInTheDocument();
    });

    it("an already-loaded picture drops the placeholder without waiting for onLoad", () => {
      stub(true, 120);
      const { container } = render(
        <Image src="/ok.jpg" alt="x" aspect="square" />,
      );
      expect(screen.getByAltText("x")).toBeInTheDocument();
      expect(container.querySelector(".animate-pulse")).toBeNull();
    });
  });
});
