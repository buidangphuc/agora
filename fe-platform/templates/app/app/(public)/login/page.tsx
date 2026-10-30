import { Button, Card, Input, Link } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
import { safeNext } from "../../../lib/session";
import { login } from "./actions";

const MESSAGES: Record<string, string> = {
  credentials: "Wrong username or password.",
  blocked: "This account cannot sign in.",
  unavailable: "Sign-in is unavailable right now. Try again in a moment.",
};

export default async function LoginPage({
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
    <Page title="Sign in" width="narrow">
      <Card>
        <form action={login} className="flex flex-col gap-6">
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
            autoComplete="current-password"
            required
          />
          <Button type="submit" variant="primary" size="lg">
            Sign in
          </Button>
        </form>
      </Card>
      <Link href={`/register?next=${encodeURIComponent(next)}`}>
        Create an account
      </Link>
    </Page>
  );
}
