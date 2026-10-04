import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import AccountLoading from "@/app/account/addresses/loading";
import AccountError from "@/app/account/error";
import FollowingLoading from "@/app/account/following/loading";
import ReferralLoading from "@/app/account/referral/loading";
import SecurityLoading from "@/app/account/security/loading";
import VerificationLoading from "@/app/account/verification/loading";
import FavoritesError from "@/app/favorites/error";
import FavoritesLoading from "@/app/favorites/loading";
import LoginLoading from "@/app/login/loading";
import NotificationsError from "@/app/notifications/error";
import NotificationsLoading from "@/app/notifications/loading";
import RegisterLoading from "@/app/register/loading";

const LOADERS: Record<string, () => React.JSX.Element> = {
  addresses: AccountLoading,
  security: SecurityLoading,
  verification: VerificationLoading,
  referral: ReferralLoading,
  following: FollowingLoading,
  favorites: FavoritesLoading,
  notifications: NotificationsLoading,
  login: LoginLoading,
  register: RegisterLoading,
};

describe("account route loading states", () => {
  for (const [name, Loading] of Object.entries(LOADERS)) {
    it(`/${name} renders a busy skeleton`, () => {
      render(<Loading />);
      const shell = screen.getByTestId("page-skeleton");
      expect(shell).toHaveAttribute("aria-busy", "true");
      expect(shell.querySelectorAll(".animate-pulse").length).toBeGreaterThan(
        1,
      );
    });
  }
});

describe("account route error states", () => {
  const BOUNDARIES = {
    account: AccountError,
    favorites: FavoritesError,
    notifications: NotificationsError,
  };
  for (const [name, ErrorPage] of Object.entries(BOUNDARIES)) {
    it(`${name} error shows a Result and retries through reset()`, () => {
      const reset = vi.fn();
      render(<ErrorPage error={new Error("boom")} reset={reset} />);
      expect(screen.getByText("Đã có lỗi xảy ra")).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
      expect(reset).toHaveBeenCalledTimes(1);
    });
  }
});
