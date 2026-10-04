"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";

import { Tabs } from "@/components/ui/Tabs";
import { useReturnState } from "./ReturnState";
import { type OrderDetailTab, parseDetailTab } from "./detailTab";

/**
 * Hành trình / Trả hàng tabs of the order detail. Both panels stay mounted so
 * the timeline fetch and the return state survive a tab switch; the active tab
 * is mirrored to `?tab=` with history.replaceState (no server round trip).
 */
export function OrderDetailTabs({
  initialTab,
  timeline,
  returns,
}: {
  initialTab: OrderDetailTab;
  timeline: React.ReactNode;
  returns: React.ReactNode;
}) {
  const [active, setActive] = useState<OrderDetailTab>(initialTab);

  const change = useCallback((id: string) => {
    const next = parseDetailTab(id);
    setActive(next);
    const url = new URL(window.location.href);
    if (next === "timeline") url.searchParams.delete("tab");
    else url.searchParams.set("tab", next);
    window.history.replaceState(null, "", `${url.pathname}${url.search}`);
  }, []);

  // A return requested from the header Modal shows up in the returns panel: bring that
  // panel forward so the buyer sees the "pending" badge instead of an unchanged timeline.
  const [ret] = useReturnState();
  const hadReturn = useRef(ret !== null);
  useEffect(() => {
    if (ret !== null && !hadReturn.current) change("returns");
    hadReturn.current = ret !== null;
  }, [ret, change]);

  return (
    <Tabs
      activeId={active}
      onChange={change}
      items={[
        { id: "timeline", label: "Hành trình", content: timeline },
        { id: "returns", label: "Trả hàng / Hoàn tiền", content: returns },
      ]}
    />
  );
}
