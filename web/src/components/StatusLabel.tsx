import { Label } from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  QuestionCircleIcon,
  BanIcon,
} from "@patternfly/react-icons";

interface StatusLabelProps {
  status: string;
}

export default function StatusLabel({ status }: StatusLabelProps) {
  const s = status.toLowerCase();

  if (s === "passed" || s === "succeeded" || s === "closed" || s === "verified") {
    return (
      <Label color="green" icon={<CheckCircleIcon />}>
        {status}
      </Label>
    );
  }
  if (s === "failed" || s === "error") {
    return (
      <Label color="red" icon={<ExclamationCircleIcon />}>
        {status}
      </Label>
    );
  }
  if (s === "pending" || s === "in progress" || s === "progressing") {
    return (
      <Label color="yellow" icon={<ExclamationTriangleIcon />}>
        {status}
      </Label>
    );
  }
  if (s === "skipped" || s === "not_configured") {
    return (
      <Label color="grey" icon={<BanIcon />}>
        {status}
      </Label>
    );
  }
  return (
    <Label color="grey" icon={<QuestionCircleIcon />}>
      {status}
    </Label>
  );
}
