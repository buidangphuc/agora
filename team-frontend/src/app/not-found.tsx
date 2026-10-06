import { ConsumerShell } from "@/components/shell/ConsumerShell";
import { NotFoundResult } from "@/components/shell/NotFoundResult";

/**
 * Root not-found replaces the whole (shop) subtree it was thrown from (including
 * that group's layout), and also serves URLs that match no route, so it mounts
 * the consumer chrome itself.
 */
export default function NotFound() {
  return (
    <ConsumerShell>
      <NotFoundResult />
    </ConsumerShell>
  );
}
