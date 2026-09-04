import { redirect } from "next/navigation";

export default function ChatThreadRedirect({
  params,
}: {
  params: { id: string };
}) {
  redirect(`/chat?thread=${encodeURIComponent(params.id)}`);
}
