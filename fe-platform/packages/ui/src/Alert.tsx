import { AlertClosable } from "./AlertClosable";
import { AlertBody, type AlertBodyProps } from "./alertBody";

export type { AlertTone } from "./alertBody";

export type AlertProps = AlertBodyProps & {
  closable?: boolean;
  onClose?: () => void;
  closeLabel?: string;
};

/** Inline status message in four tones; `danger` is role=alert, the others role=status; server-safe unless `closable`. */
export function Alert({
  closable = false,
  onClose,
  closeLabel = "Dismiss",
  ...body
}: AlertProps) {
  if (closable)
    return (
      <AlertClosable {...body} onClose={onClose} closeLabel={closeLabel} />
    );
  return <AlertBody {...body} />;
}
