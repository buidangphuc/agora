import { redirect } from "next/navigation";

import { ADMIN_HOME } from "@/components/shell/nav";

export default function HomePage() {
  redirect(ADMIN_HOME);
}
