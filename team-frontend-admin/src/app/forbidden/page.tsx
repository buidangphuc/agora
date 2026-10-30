import type { Metadata } from "next";

import { ForbiddenView } from "@/components/shell/ForbiddenView";

export const metadata: Metadata = {
  title: "Không có quyền",
  referrer: "no-referrer",
};

export default function ForbiddenPage() {
  return <ForbiddenView />;
}
