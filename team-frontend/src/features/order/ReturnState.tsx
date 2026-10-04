"use client";

import React, { createContext, useContext, useState } from "react";

import type { ViewOrderReturn } from "@/lib/gateway/orders";

interface ReturnStateValue {
  ret: ViewOrderReturn | null;
  setRet: (ret: ViewOrderReturn) => void;
}

const ReturnStateContext = createContext<ReturnStateValue | null>(null);

/**
 * Shares the order's return request between the header trigger and the
 * returns section. The gateway has no "get return by order" call, so the
 * result of the create / refund action is the only source for it.
 */
export function ReturnStateProvider({
  initialReturn = null,
  children,
}: {
  initialReturn?: ViewOrderReturn | null;
  children: React.ReactNode;
}) {
  const [ret, setRet] = useState<ViewOrderReturn | null>(initialReturn);
  return (
    <ReturnStateContext.Provider value={{ ret, setRet }}>
      {children}
    </ReturnStateContext.Provider>
  );
}

/** The shared return when inside a provider, otherwise local state. */
export function useReturnState(
  initialReturn: ViewOrderReturn | null = null,
): [ViewOrderReturn | null, (ret: ViewOrderReturn) => void] {
  const shared = useContext(ReturnStateContext);
  const local = useState<ViewOrderReturn | null>(initialReturn);
  return shared ? [shared.ret, shared.setRet] : local;
}
