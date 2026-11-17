import type { InsightParams, fetchInsights } from "@/lib/gateway/insights";

import { AiSurfacesPanel } from "./AiSurfacesPanel";
import { ExperimentsPanel } from "./ExperimentsPanel";
import { PlacementsPanel } from "./PlacementsPanel";
import { RetentionPanel } from "./RetentionPanel";

export function InsightsSection({
  insights,
  params,
  flagText,
}: {
  insights: Awaited<ReturnType<typeof fetchInsights>>;
  params: InsightParams;
  flagText: string;
}) {
  return (
    <div>
      <PlacementsPanel
        recommendation={insights.recommendation}
        search={insights.search}
        windowHours={params.windowHours}
      />
      <RetentionPanel result={insights.retention} weeks={params.weeks} />
      <ExperimentsPanel
        result={insights.experiment}
        flag={flagText}
        rangeDays={params.rangeDays}
      />
      <AiSurfacesPanel
        result={insights.aiSurfaces}
        rangeDays={params.rangeDays}
      />
    </div>
  );
}
