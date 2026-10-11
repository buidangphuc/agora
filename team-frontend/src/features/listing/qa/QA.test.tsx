import { act, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { setupUser } from "@/test/user";
import { QAAnswerForm } from "./QAAnswerForm";
import { QAAskForm } from "./QAAskForm";
import { QASection } from "./QASection";
import { answerQuestionAction, askQuestionAction } from "./actions";

vi.mock("./actions", () => ({
  askQuestionAction: vi.fn(),
  answerQuestionAction: vi.fn(),
}));
// The list streams in its own Suspense boundary; stub it for the section shell.
vi.mock("./QuestionList", () => ({
  QuestionList: () => <div data-testid="list" />,
}));

function deferred<T>() {
  let resolve: (v: T) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

beforeEach(() => vi.clearAllMocks());

describe("QAAskForm", () => {
  it("disables submit while the text is empty and makes no request", async () => {
    const user = setupUser();
    render(
      <ToastProvider>
        <QAAskForm listingId="L" />
      </ToastProvider>,
    );
    const submit = screen.getByRole("button", { name: "Đặt câu hỏi cho Shop" });
    expect(submit).toBeDisabled();
    await user.type(screen.getByLabelText("Câu hỏi của bạn"), "   ");
    expect(submit).toBeDisabled();
    await user.click(submit);
    expect(askQuestionAction).not.toHaveBeenCalled();
    expect(screen.getByTestId("qa-ask-form")).toBeInTheDocument();
  });

  it("shows pending, then a success toast, and clears the field", async () => {
    const user = setupUser();
    const d = deferred<{ ok: true }>();
    vi.mocked(askQuestionAction).mockReturnValue(d.promise);
    render(
      <ToastProvider>
        <QAAskForm listingId="L" />
      </ToastProvider>,
    );
    const field = screen.getByLabelText("Câu hỏi của bạn");
    await user.type(field, "Còn bảo hành không?");
    await user.click(
      screen.getByRole("button", { name: "Đặt câu hỏi cho Shop" }),
    );

    expect(askQuestionAction).toHaveBeenCalledWith("L", "Còn bảo hành không?");
    const submit = screen.getByRole("button", { name: "Đặt câu hỏi cho Shop" });
    expect(submit).toHaveAttribute("aria-busy", "true");
    expect(field).toBeDisabled();

    await act(async () => {
      d.resolve({ ok: true });
    });
    expect(
      await screen.findByText("Đã gửi câu hỏi tới shop!"),
    ).toBeInTheDocument();
    expect(field).toHaveValue("");
    expect(field).toBeEnabled();
  });

  it("reports a failure with an error toast and an inline Alert, keeping the text", async () => {
    const user = setupUser();
    vi.mocked(askQuestionAction).mockResolvedValue({
      ok: false,
      error: "Quá nhiều yêu cầu",
    });
    render(
      <ToastProvider>
        <QAAskForm listingId="L" />
      </ToastProvider>,
    );
    await user.type(screen.getByLabelText("Câu hỏi của bạn"), "Hỏi gì đó");
    await user.click(
      screen.getByRole("button", { name: "Đặt câu hỏi cho Shop" }),
    );
    // The inline Alert and the error toast both announce the failure.
    const alerts = await screen.findAllByRole("alert");
    expect(alerts).toHaveLength(2);
    for (const a of alerts) expect(a).toHaveTextContent("Quá nhiều yêu cầu");
    expect(screen.getByLabelText("Câu hỏi của bạn")).toHaveValue("Hỏi gì đó");
  });
});

describe("QAAnswerForm", () => {
  it("opens from the toggle, blocks empty submits, and answers with a toast", async () => {
    const user = setupUser();
    vi.mocked(answerQuestionAction).mockResolvedValue({ ok: true });
    render(
      <ToastProvider>
        <QAAnswerForm listingId="L" questionId="q1" />
      </ToastProvider>,
    );
    await user.click(screen.getByTestId("qa-answer-toggle"));
    const send = screen.getByRole("button", { name: "Gửi trả lời" });
    expect(send).toBeDisabled();
    await user.type(screen.getByLabelText("Nội dung trả lời"), "Còn ạ");
    await user.click(send);
    expect(answerQuestionAction).toHaveBeenCalledWith("L", "q1", "Còn ạ");
    expect(await screen.findByText("Đã gửi câu trả lời!")).toBeInTheDocument();
    expect(screen.getByTestId("qa-answer-toggle")).toBeInTheDocument();
  });

  it("shows an error toast and an inline Alert when answering fails", async () => {
    const user = setupUser();
    vi.mocked(answerQuestionAction).mockResolvedValue({
      ok: false,
      error: "Không được",
    });
    render(
      <ToastProvider>
        <QAAnswerForm listingId="L" questionId="q1" />
      </ToastProvider>,
    );
    await user.click(screen.getByTestId("qa-answer-toggle"));
    await user.type(screen.getByLabelText("Nội dung trả lời"), "x");
    await user.click(screen.getByRole("button", { name: "Gửi trả lời" }));
    const alerts = await screen.findAllByRole("alert");
    expect(alerts).toHaveLength(2); // inline Alert + error toast
    for (const a of alerts) expect(a).toHaveTextContent("Không được");
    expect(screen.getByRole("button", { name: "Gửi trả lời" })).toBeEnabled();
  });

  it("can be cancelled back to the toggle", async () => {
    const user = setupUser();
    render(
      <ToastProvider>
        <QAAnswerForm listingId="L" questionId="q1" />
      </ToastProvider>,
    );
    await user.click(screen.getByTestId("qa-answer-toggle"));
    await user.click(screen.getByRole("button", { name: "Hủy" }));
    expect(screen.getByTestId("qa-answer-toggle")).toBeInTheDocument();
    expect(screen.queryByLabelText("Nội dung trả lời")).toBeNull();
  });
});

describe("QASection", () => {
  it("shows the ask form for a logged-in buyer", () => {
    render(
      <ToastProvider>
        <QASection listingId="L" loggedIn />
      </ToastProvider>,
    );
    expect(document.getElementById("qa")).not.toBeNull();
    expect(screen.getByTestId("qa-ask-form")).toBeInTheDocument();
    expect(screen.queryByTestId("qa-login-prompt")).toBeNull();
  });

  it("prompts a guest to log in with a returnUrl and renders no ask form", () => {
    render(<QASection listingId="L" loggedIn={false} />);
    expect(screen.queryByTestId("qa-ask-form")).toBeNull();
    const prompt = screen.getByTestId("qa-login-prompt");
    expect(
      within(prompt).getByRole("link", { name: "Đăng nhập" }),
    ).toHaveAttribute("href", "/login?returnUrl=/listing/L");
  });
});
