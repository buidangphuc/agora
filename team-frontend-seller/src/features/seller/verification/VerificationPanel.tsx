import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import { KycUploadForm } from "./KycUploadForm";
import { rejectReasonText, verificationStatusLabel } from "./status";

export function VerificationPanel({
  status,
  rejectReasonCode = "",
}: {
  status: VerificationStatus;
  rejectReasonCode?: string;
}) {
  const canSubmit =
    status !== VerificationStatus.PENDING &&
    status !== VerificationStatus.VERIFIED;
  return (
    <Card>
      <CardContent>
        <div className="space-y-4">
          <p className="text-lg font-semibold">
            {verificationStatusLabel(status)}
          </p>
          {status === VerificationStatus.REJECTED && (
            <Alert
              type="error"
              description={rejectReasonText(rejectReasonCode)}
            />
          )}
          {canSubmit && <KycUploadForm />}
        </div>
      </CardContent>
    </Card>
  );
}
