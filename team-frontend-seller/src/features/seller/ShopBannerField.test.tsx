import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getUploadUrlAction } from "@/features/listing/actions";
import { ShopBannerField } from "./ShopBannerField";

vi.mock("@/features/listing/actions", () => ({
  getUploadUrlAction: vi.fn(),
}));

const MB = 1024 * 1024;
const fetchMock = vi.fn();

function Harness({
  initial = "",
  onValue,
}: {
  initial?: string;
  onValue?: (v: string) => void;
}) {
  const [v, setV] = useState(initial);
  return (
    <ShopBannerField
      value={v}
      onChange={(next: string) => {
        setV(next);
        onValue?.(next);
      }}
    />
  );
}

const fileOf = (size: number, type: string, name = "b.bin") =>
  new File([new Uint8Array(size)], name, { type });
const pick = (f: File) =>
  fireEvent.change(screen.getByLabelText(/Banner/), { target: { files: [f] } });
const preview = () => screen.queryByRole("img", { name: /Banner/ });

beforeEach(() => {
  vi.clearAllMocks();
  fetchMock.mockResolvedValue({ ok: true, status: 200 });
  vi.stubGlobal("fetch", fetchMock);
  vi.mocked(getUploadUrlAction).mockResolvedValue({
    ok: true,
    message: "",
    uploadUrl: "http://put/x",
    imageKey: "k1",
    publicUrl: "http://pub/banner-new.png",
  } as never);
});
afterEach(() => vi.unstubAllGlobals());

describe("ShopBannerField", () => {
  it("Seller shop UI: an accepted banner is uploaded and previewed — uploads and previews", async () => {
    const onValue = vi.fn();
    render(<Harness onValue={onValue} />);
    pick(fileOf(2 * MB, "image/png", "a.png"));
    await waitFor(() =>
      expect(preview()).toHaveAttribute("src", "http://pub/banner-new.png"),
    );
    expect(getUploadUrlAction).toHaveBeenCalledTimes(1);
    expect(vi.mocked(getUploadUrlAction).mock.calls[0][0]).toBe("image/png");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("http://put/x");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "PUT" });
    expect(onValue).toHaveBeenLastCalledWith("http://pub/banner-new.png");
  });

  it("Seller shop UI: a wrong type or oversize banner is refused before upload — type and size refused", async () => {
    const onValue = vi.fn();
    render(<Harness onValue={onValue} />);
    pick(fileOf(1024, "application/pdf", "a.pdf"));
    expect(
      await screen.findByText("Chỉ chấp nhận ảnh JPEG, PNG hoặc WebP."),
    ).toBeInTheDocument();
    pick(fileOf(10 * MB + 1, "image/png", "big.png"));
    expect(
      await screen.findByText("Kích thước file không vượt quá 10MB."),
    ).toBeInTheDocument();
    expect(getUploadUrlAction).toHaveBeenCalledTimes(0);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(onValue).not.toHaveBeenCalled();
  });

  it("Seller shop UI: a file of exactly 10 MB is accepted — 10 MB boundary accepted", async () => {
    render(<Harness />);
    pick(fileOf(10 * MB, "image/webp", "ok.webp"));
    await waitFor(() => expect(getUploadUrlAction).toHaveBeenCalledTimes(1));
    expect(vi.mocked(getUploadUrlAction).mock.calls[0][0]).toBe("image/webp");
  });

  it("Seller shop UI: a failed upload keeps the previous banner — failed upload keeps previous banner", async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 500 });
    const onValue = vi.fn();
    render(<Harness initial="http://pub/B.png" onValue={onValue} />);
    pick(fileOf(1024, "image/png", "a.png"));
    expect(await screen.findByText(/^Upload thất bại/)).toBeInTheDocument();
    expect(preview()).toHaveAttribute("src", "http://pub/B.png");
    expect(onValue).not.toHaveBeenCalled();
  });

  it("Seller shop UI: removing the banner clears it — remove clears value", async () => {
    const onValue = vi.fn();
    render(<Harness initial="http://pub/B.png" onValue={onValue} />);
    fireEvent.click(screen.getByRole("button", { name: "Xóa banner" }));
    expect(onValue).toHaveBeenLastCalledWith("");
    await waitFor(() => expect(preview()).toBeNull());
  });
});
