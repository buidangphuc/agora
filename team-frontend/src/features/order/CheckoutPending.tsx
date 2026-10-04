"use client";

import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";

interface CheckoutPendingValue {
  /** True from the moment "Đặt hàng" is activated until it fails (stays true after success). */
  pending: boolean;
  setPending: (next: boolean) => void;
}

const CheckoutPendingContext = createContext<CheckoutPendingValue>({
  pending: false,
  setPending: () => {},
});

/** Shares the place-order pending flag with the Stepper and Back links. */
export function CheckoutPendingProvider({ children }: { children: ReactNode }) {
  const [pending, setPendingState] = useState(false);
  const setPending = useCallback((next: boolean) => setPendingState(next), []);
  const value = useMemo(() => ({ pending, setPending }), [pending, setPending]);
  return (
    <CheckoutPendingContext.Provider value={value}>
      {children}
    </CheckoutPendingContext.Provider>
  );
}

export function useCheckoutPending(): CheckoutPendingValue {
  return useContext(CheckoutPendingContext);
}
