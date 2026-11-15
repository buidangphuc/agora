import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import { VerificationPanel } from "./VerificationPanel";

vi.mock("./actions", () => ({
  createKycUploadUrlAction: vi.fn(),
  submitKycAction: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());

const fileInputs = (c: HTMLElement) =>
  c.querySelectorAll('input[type="file"]').length;

describe("VerificationPanel", () => {
  it("A seller who never submitted sees NOT_SUBMITTED and the form", () => {
    const { container } = render(
      <VerificationPanel status={VerificationStatus.NOT_SUBMITTED} />,
    );
    expect(screen.getByText("Chưa gửi hồ sơ")).toBeInTheDocument();
    expect(fileInputs(container)).toBe(1);
  });

  it("A pending submission hides the form", () => {
    const { container } = render(
      <VerificationPanel status={VerificationStatus.PENDING} />,
    );
    expect(screen.getByText("Đang chờ duyệt")).toBeInTheDocument();
    expect(fileInputs(container)).toBe(0);
  });

  it("A verified seller sees the verified state", () => {
    const { container } = render(
      <VerificationPanel status={VerificationStatus.VERIFIED} />,
    );
    expect(screen.getByText("Đã xác minh")).toBeInTheDocument();
    expect(fileInputs(container)).toBe(0);
  });

  it("A rejected submission shows the reason and allows resubmission", () => {
    const { container } = render(
      <VerificationPanel
        status={VerificationStatus.REJECTED}
        rejectReasonCode="document_unreadable"
      />,
    );
    expect(screen.getByText("Bị từ chối")).toBeInTheDocument();
    expect(screen.queryByText(/document_unreadable/)).toBeNull();
    expect(fileInputs(container)).toBe(1);
  });

  it("An unknown reject reason code falls back to a generic text and never renders the raw code", () => {
    render(
      <VerificationPanel
        status={VerificationStatus.REJECTED}
        rejectReasonCode="zzz_unknown_code"
      />,
    );
    expect(
      screen.getByText("Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/zzz_unknown_code/)).toBeNull();
  });
});
