import { Button, Card, Input, Link } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
import { safeNext } from "../../../lib/session";
import { register } from "./actions";

const MESSAGES: Record<string, string> = {
  taken: "That username or password was not accepted. Try another.",
  blocked: "This account cannot be created.",
  credentials: "That username or password was not accepted. Try another.",
  unavailable: "Registration is unavailable right now. Try again in a moment.",
};

export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; error?: string }>;
}) {
  const sp = await searchParams;
  const next = safeNext(sp.next);
  const message = sp.error
    ? (MESSAGES[sp.error] ?? MESSAGES.unavailable)
    : undefined;
  return (
    <Page title="Create an account" width="narrow">
      <Card>
        <form action={register} className="flex flex-col gap-6">
          <input type="hidden" name="next" value={next} />
          {message ? (
            <p role="alert" className="text-text-danger">
              {message}
            </p>
          ) : null}
          <Input
            label="Username"
            name="username"
            autoComplete="username"
            required
          />
          <Input
            label="Password"
            name="password"
            type="password"
            autoComplete="new-password"
            required
          />
          <Button type="submit" variant="primary" size="lg">
            Create account
          </Button>
        </form>
      </Card>
      <Link href={`/login?next=${encodeURIComponent(next)}`}>
        I already have an account
      </Link>
    </Page>
  );
}
