import { redirect } from "next/navigation";

export default function ShopIndex(): never {
  redirect("/products");
}
