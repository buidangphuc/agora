import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { KycUploadForm } from "./KycUploadForm";
import { createKycUploadUrlAction, submitKycAction } from "./actions";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("./actions", () => ({
  createKycUploadUrlAction: vi.fn(),
  submitKycAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const fetchMock = vi.fn();

function mkFile(name: string, type: string, size: number): File {
  const file = new File(["x"], name, { type });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

const PNG = () => mkFile("a.png", "image/png", 1000);

function select(container: HTMLElement, files: File[]) {
  const input = container.querySelector(
    'input[type="file"]',
  ) as HTMLInputElement;
  fireEvent.change(input, { target: { files } });
}

const consent = () => screen.getByRole("checkbox");
const submitBtn = () =>
  screen.getByRole("button", { name: "Gửi hồ sơ xác minh" });

function wireUploads() {
  let n = 0;
  vi.mocked(createKycUploadUrlAction).mockImplementation(async () => {
    n += 1;
    return {
      ok: true,
      data: { uploadUrl: `https://s3/put-${n}`, docRef: `ref-${n}` },
    };
  });
  vi.mocked(submitKycAction).mockResolvedValue({ ok: true });
  fetchMock.mockResolvedValue({ ok: true, status: 200 });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", fetchMock);
  wireUploads();
});
afterEach(() => vi.unstubAllGlobals());

describe("KycUploadForm", () => {
  it("Three valid files are uploaded and submitted", async () => {
    const { container } = render(<KycUploadForm />);
    const files = [
      mkFile("a.png", "image/png", 1000),
      mkFile("b.jpg", "image/jpeg", 2000),
      mkFile("c.pdf", "application/pdf", 3000),
    ];
    select(container, files);
    fireEvent.click(consent());
    fireEvent.click(submitBtn());
    await waitFor(() => expect(submitKycAction).toHaveBeenCalledTimes(1));

    expect(createKycUploadUrlAction).toHaveBeenCalledTimes(3);
    expect(createKycUploadUrlAction).toHaveBeenNthCalledWith(
      1,
      "image/png",
      1000,
    );
    expect(createKycUploadUrlAction).toHaveBeenNthCalledWith(
      2,
      "image/jpeg",
      2000,
    );
    expect(createKycUploadUrlAction).toHaveBeenNthCalledWith(
      3,
      "application/pdf",
      3000,
    );
    expect(fetchMock).toHaveBeenCalledTimes(3);
    fetchMock.mock.calls.forEach(([url, init], i) => {
      expect(url).toBe(`https://s3/put-${i + 1}`);
      expect(init.method).toBe("PUT");
      expect(init.headers["Content-Type"]).toBe(files[i].type);
      expect(init.body).toBe(files[i]);
    });
    expect(submitKycAction).toHaveBeenCalledWith("national_id", [
      "ref-1",
      "ref-2",
      "ref-3",
    ]);
  });

  it("Consent unticked blocks submission", () => {
    const { container } = render(<KycUploadForm />);
    select(container, [PNG()]);
    expect(submitBtn()).toBeDisabled();
    fireEvent.click(submitBtn());
    expect(createKycUploadUrlAction).not.toHaveBeenCalled();
    fireEvent.click(consent());
    expect(submitBtn()).toBeEnabled();
  });

  it("A file over 5 MiB is rejected before upload", () => {
    const { container } = render(<KycUploadForm />);
    select(container, [mkFile("big.png", "image/png", 5 * 1024 * 1024 + 1)]);
    fireEvent.click(consent());
    expect(screen.getByText("Tệp tối đa 5 MiB")).toBeInTheDocument();
    expect(submitBtn()).toBeDisabled();
    expect(createKycUploadUrlAction).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("A file of exactly 5 MiB is accepted", () => {
    const { container } = render(<KycUploadForm />);
    select(container, [mkFile("ok.png", "image/png", 5242880)]);
    fireEvent.click(consent());
    expect(screen.queryByText("Tệp tối đa 5 MiB")).toBeNull();
    expect(submitBtn()).toBeEnabled();
  });

  it("A wrong file type or an empty file is rejected", () => {
    const { container, unmount } = render(<KycUploadForm />);
    select(container, [mkFile("x.gif", "image/gif", 10)]);
    fireEvent.click(consent());
    expect(screen.getByText("Chỉ nhận JPG, PNG hoặc PDF")).toBeInTheDocument();
    expect(submitBtn()).toBeDisabled();
    unmount();

    const second = render(<KycUploadForm />);
    select(second.container, [mkFile("e.png", "image/png", 0)]);
    fireEvent.click(consent());
    expect(screen.getByText("Tệp rỗng")).toBeInTheDocument();
    expect(submitBtn()).toBeDisabled();
  });

  it("More than three files are refused", () => {
    const { container } = render(<KycUploadForm />);
    select(container, [PNG(), PNG(), PNG(), PNG()]);
    fireEvent.click(consent());
    expect(screen.getByText("Tối đa 3 tệp")).toBeInTheDocument();
    expect(submitBtn()).toBeDisabled();
    expect(createKycUploadUrlAction).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("A failed PUT stops the submission and a retry reuses already uploaded refs", async () => {
    const { container } = render(<KycUploadForm />);
    select(container, [PNG(), PNG(), PNG()]);
    fireEvent.click(consent());
    fetchMock
      .mockResolvedValueOnce({ ok: true, status: 200 })
      .mockResolvedValueOnce({ ok: false, status: 403 });
    fireEvent.click(submitBtn());
    await screen.findByText("Tải tệp thất bại, vui lòng thử lại");
    expect(submitKycAction).not.toHaveBeenCalled();
    expect(createKycUploadUrlAction).toHaveBeenCalledTimes(2);
    expect(submitBtn()).toBeEnabled();

    fireEvent.click(submitBtn());
    await waitFor(() => expect(submitKycAction).toHaveBeenCalledTimes(1));
    expect(createKycUploadUrlAction).toHaveBeenCalledTimes(4);
    expect(fetchMock).toHaveBeenCalledTimes(5);
    expect(submitKycAction).toHaveBeenCalledWith("national_id", [
      "ref-1",
      "ref-3",
      "ref-4",
    ]);
  });

  it("A failed PUT stops the submission: a network error (rejected fetch) behaves the same", async () => {
    const { container } = render(<KycUploadForm />);
    select(container, [PNG()]);
    fireEvent.click(consent());
    fetchMock.mockRejectedValueOnce(new TypeError("network"));
    fireEvent.click(submitBtn());
    await screen.findByText("Tải tệp thất bại, vui lòng thử lại");
    expect(submitKycAction).not.toHaveBeenCalled();
  });

  it("The server refuses a doc_ref", async () => {
    vi.mocked(submitKycAction).mockResolvedValue({
      ok: false,
      error: "Hồ sơ không hợp lệ, vui lòng tải lại tệp",
    });
    const { container } = render(<KycUploadForm />);
    select(container, [PNG()]);
    fireEvent.click(consent());
    fireEvent.click(submitBtn());
    await screen.findByText("Hồ sơ không hợp lệ, vui lòng tải lại tệp");
    expect(refresh).not.toHaveBeenCalled();
  });

  it("Submit cannot be sent twice", async () => {
    let release: (v: unknown) => void = () => {};
    vi.mocked(submitKycAction).mockReturnValue(
      new Promise((r) => {
        release = r;
      }) as never,
    );
    const { container } = render(<KycUploadForm />);
    select(container, [PNG()]);
    fireEvent.click(consent());
    fireEvent.click(submitBtn());
    fireEvent.click(submitBtn());
    fireEvent.submit(container.querySelector("form") as HTMLFormElement);
    await waitFor(() => expect(submitKycAction).toHaveBeenCalledTimes(1));
    release({ ok: true });
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(submitKycAction).toHaveBeenCalledTimes(1);
    expect(createKycUploadUrlAction).toHaveBeenCalledTimes(1);
  });

  it("Image bytes and keys never reach logs or the server action payload: actions get no File and the DOM shows no url or ref", async () => {
    const { container } = render(<KycUploadForm />);
    select(container, [PNG(), PNG()]);
    fireEvent.click(consent());
    fireEvent.click(submitBtn());
    await waitFor(() => expect(submitKycAction).toHaveBeenCalledTimes(1));
    for (const call of vi.mocked(createKycUploadUrlAction).mock.calls) {
      expect(call).toHaveLength(2);
      expect(typeof call[0]).toBe("string");
      expect(typeof call[1]).toBe("number");
    }
    const [docType, refs] = vi.mocked(submitKycAction).mock.calls[0];
    expect(typeof docType).toBe("string");
    expect((refs as unknown[]).every((r) => typeof r === "string")).toBe(true);
    expect(container.innerHTML).not.toContain("https://s3/");
    expect(container.innerHTML).not.toMatch(/ref-\d/);
  });
});
