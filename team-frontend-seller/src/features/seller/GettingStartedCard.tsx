import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import {
  type GettingStartedSources,
  deriveGettingStarted,
} from "./gettingStarted";

export async function GettingStartedCard({
  sources,
}: { sources: GettingStartedSources }) {
  const { steps, allDone } = await deriveGettingStarted(sources);
  if (allDone) return null;
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Bắt đầu bán hàng</CardTitle>
          <CardDescription>
            Hoàn tất các bước sau để gian hàng sẵn sàng.
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        <ul className="flex flex-col gap-3">
          {steps.map((s) => (
            <li
              key={s.label}
              data-state={s.state}
              className="flex items-center justify-between gap-3"
            >
              <span>{s.label}</span>
              {s.state === "done" ? (
                <span aria-label="Đã xong">✓</span>
              ) : (
                <span className="flex items-center gap-2">
                  {s.state === "unknown" ? <span>chưa rõ</span> : null}
                  <Link href={s.href} variant="outline" size="sm">
                    Thực hiện
                  </Link>
                </span>
              )}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
