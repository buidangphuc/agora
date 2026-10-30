import { redirect } from "next/navigation";

// The seller flow moved to the seller center.
export default function SellPage() {
  redirect("/seller/new");
}
