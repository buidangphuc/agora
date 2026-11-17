"use client";

import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";

import { toggleSelection } from "./bulkShip";

interface SelectionValue {
  selected: string[];
  toggle: (id: string) => void;
  setSelected: (ids: string[]) => void;
  error: string;
}

const noop = () => undefined;
const Ctx = createContext<SelectionValue>({
  selected: [],
  toggle: noop,
  setSelected: noop,
  error: "",
});

export function BulkShipSelectionProvider({
  children,
}: { children: ReactNode }) {
  const [selected, setSelectedState] = useState<string[]>([]);
  const [error, setError] = useState("");

  const toggle = useCallback((id: string) => {
    setSelectedState((cur) => {
      const next = toggleSelection(cur, id);
      setError(next.error ?? "");
      return next.selected;
    });
  }, []);
  const setSelected = useCallback((ids: string[]) => {
    setError("");
    setSelectedState(ids);
  }, []);

  const value = useMemo(
    () => ({ selected, toggle, setSelected, error }),
    [selected, toggle, setSelected, error],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useBulkShipSelection(): SelectionValue {
  return useContext(Ctx);
}

export function OrderSelectCheckbox({ orderId }: { orderId: string }) {
  const { selected, toggle } = useBulkShipSelection();
  return (
    <input
      type="checkbox"
      aria-label={`Chọn đơn #${orderId.slice(0, 8)}`}
      checked={selected.includes(orderId)}
      onChange={() => toggle(orderId)}
    />
  );
}
