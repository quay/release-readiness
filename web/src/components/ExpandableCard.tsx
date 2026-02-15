import { Card, CardBody, ExpandableSection } from "@patternfly/react-core";
import { useState } from "react";

export default function ExpandableCard({
	title,
	children,
}: {
	title: string;
	children: React.ReactNode;
}) {
	const [expanded, setExpanded] = useState(true);
	return (
		<Card isCompact style={{ marginBottom: "1rem" }}>
			<CardBody>
				<ExpandableSection
					toggleText={title}
					isExpanded={expanded}
					onToggle={(_e, val) => setExpanded(val)}
				>
					{children}
				</ExpandableSection>
			</CardBody>
		</Card>
	);
}
