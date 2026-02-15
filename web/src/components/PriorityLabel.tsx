import { Label } from "@patternfly/react-core";

export default function PriorityLabel({ priority }: { priority: string }) {
	const p = priority.toLowerCase();
	if (p === "critical" || p === "blocker") {
		return <Label color="red">{priority}</Label>;
	}
	if (p === "major") {
		return <Label color="yellow">{priority}</Label>;
	}
	return <Label color="grey">{priority}</Label>;
}
