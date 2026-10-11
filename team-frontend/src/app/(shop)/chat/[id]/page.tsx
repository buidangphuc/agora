import { redirect } from "next/navigation";

export default async function ChatThreadRedirect(props: {
  params: Promise<{ id: string }>;
}) {
  const params = await props.params;
  redirect(`/chat?thread=${encodeURIComponent(params.id)}`);
}
