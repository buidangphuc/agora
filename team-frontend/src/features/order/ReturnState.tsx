"use client";

import React, { createContext, useCallback, useContext, useState } from "react";

import type { ViewOrderReturn } from "@/lib/gateway/orders";

interface ReturnStateValue {
  returns: ViewOrderReturn[];
  addReturn: (ret: ViewOrderReturn) => void;
}

const ReturnStateContext = createContext<ReturnStateValue | null>(null);

/** Insert a return, or replace the one with the same id. Newest first. */
function upsert(list: ViewOrderReturn[], ret: ViewOrderReturn) {
  return list.some((r) => r.id === ret.id)
    ? list.map((r) => (r.id === ret.id ? ret : r))
    : [ret, ...list];
}

function useReturnList(initial: ViewOrderReturn[]): ReturnStateValue {
  const [returns, setReturns] = useState<ViewOrderReturn[]>(initial);
  const addReturn = useCallback(
    (ret: ViewOrderReturn) => setReturns((cur) => upsert(cur, ret)),
    [],
  );
  return { returns, addReturn };
}

/**
 * Shares the order's returns (loaded server-side through `ListOrderReturns`)
 * between the header trigger and the returns section; a return created in the
 * Modal is added to the list.
 */
export function ReturnStateProvider({
  initialReturns = [],
  children,
}: {
  initialReturns?: ViewOrderReturn[];
  children: React.ReactNode;
}) {
  const value = useReturnList(initialReturns);
  return (
    <ReturnStateContext.Provider value={value}>
      {children}
    </ReturnStateContext.Provider>
  );
}

/** The shared returns when inside a provider, otherwise local state. */
export function useReturnState(
  initialReturns: ViewOrderReturn[] = [],
): [ViewOrderReturn[], (ret: ViewOrderReturn) => void] {
  const shared = useContext(ReturnStateContext);
  const local = useReturnList(initialReturns);
  const v = shared ?? local;
  return [v.returns, v.addReturn];
}
