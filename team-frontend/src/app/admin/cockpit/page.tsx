import { CockpitView } from "@/features/admin/CockpitView";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Admin Operations Cockpit | Sàn Thương Mại Điện Tử",
  description:
    "Bản đồ vận hành thời gian thực, telemetry, Golden Signals và Jaeger Distributed Tracing.",
};

export default function CockpitPage() {
  return <CockpitView />;
}
