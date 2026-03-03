import {
	Label,
	SearchInput,
	ToggleGroup,
	ToggleGroupItem,
	Toolbar,
	ToolbarContent,
	ToolbarItem,
} from "@patternfly/react-core";
import {
	ExpandableRowContent,
	Table,
	Tbody,
	Td,
	Th,
	Thead,
	type ThProps,
	Tr,
} from "@patternfly/react-table";
import { useMemo, useState } from "react";
import type { Vulnerability } from "../api/types";

const severityWeight: Record<string, number> = {
	Critical: 0,
	High: 1,
	Medium: 2,
	Low: 3,
	Unknown: 4,
};

const severityColor: Record<string, "red" | "orange" | "yellow" | "grey"> = {
	Critical: "red",
	High: "red",
	Medium: "orange",
	Low: "yellow",
};

export default function VulnerabilitiesTable({
	vulnerabilities,
}: { vulnerabilities: Vulnerability[] }) {
	const [activeSortIndex, setActiveSortIndex] = useState<number | undefined>(
		undefined,
	);
	const [activeSortDirection, setActiveSortDirection] = useState<
		"asc" | "desc" | undefined
	>(undefined);
	const [nameFilter, setNameFilter] = useState("");
	const [severityFilters, setSeverityFilters] = useState<Set<string>>(
		new Set(),
	);
	const [expandedVulns, setExpandedVulns] = useState<Set<number>>(new Set());

	const availableSeverities = useMemo(() => {
		const sevs = new Set<string>();
		for (const v of vulnerabilities) {
			sevs.add(v.severity || "Unknown");
		}
		return [...sevs].sort(
			(a, b) => (severityWeight[a] ?? 99) - (severityWeight[b] ?? 99),
		);
	}, [vulnerabilities]);

	const processedVulns = useMemo(() => {
		let vulns = [...vulnerabilities];

		if (nameFilter) {
			const lower = nameFilter.toLowerCase();
			vulns = vulns.filter(
				(v) =>
					v.name.toLowerCase().includes(lower) ||
					v.package_name.toLowerCase().includes(lower),
			);
		}
		if (severityFilters.size > 0) {
			vulns = vulns.filter((v) =>
				severityFilters.has(v.severity || "Unknown"),
			);
		}

		if (activeSortIndex !== undefined && activeSortDirection !== undefined) {
			vulns.sort((a, b) => {
				let cmp = 0;
				switch (activeSortIndex) {
					case 1:
						cmp = a.name.localeCompare(b.name);
						break;
					case 2:
						cmp =
							(severityWeight[a.severity] ?? 99) -
							(severityWeight[b.severity] ?? 99);
						break;
					case 3:
						cmp = a.package_name.localeCompare(b.package_name);
						break;
				}
				return activeSortDirection === "asc" ? cmp : -cmp;
			});
		}

		return vulns;
	}, [
		vulnerabilities,
		nameFilter,
		severityFilters,
		activeSortIndex,
		activeSortDirection,
	]);

	const getSortParams = (columnIndex: number): ThProps["sort"] => ({
		sortBy: {
			index: activeSortIndex,
			direction: activeSortDirection,
		},
		onSort: (_event, index, direction) => {
			setActiveSortIndex(index);
			setActiveSortDirection(direction);
		},
		columnIndex,
	});

	return (
		<>
			<Toolbar>
				<ToolbarContent>
					<ToolbarItem>
						<SearchInput
							placeholder="Filter by CVE or package..."
							value={nameFilter}
							onChange={(_e, val) => setNameFilter(val)}
							onClear={() => setNameFilter("")}
						/>
					</ToolbarItem>
					<ToolbarItem>
						<ToggleGroup aria-label="Severity filter">
							{availableSeverities.map((s) => (
								<ToggleGroupItem
									key={s}
									text={s}
									isSelected={severityFilters.has(s)}
									onChange={() => {
										setSeverityFilters((prev) => {
											const next = new Set(prev);
											if (next.has(s)) {
												next.delete(s);
											} else {
												next.add(s);
											}
											return next;
										});
									}}
								/>
							))}
						</ToggleGroup>
					</ToolbarItem>
				</ToolbarContent>
			</Toolbar>
			{(nameFilter || severityFilters.size > 0) && (
				<div
					style={{
						fontSize: "0.85em",
						padding: "0.25rem 0",
						color: "var(--pf-t--global--text--color--subtle)",
					}}
				>
					{processedVulns.length} of {vulnerabilities.length}{" "}
					vulnerabilities
				</div>
			)}
			<Table variant="compact" borders={false}>
				<Thead>
					<Tr>
						<Th screenReaderText="Toggle" />
						<Th sort={getSortParams(1)}>CVE</Th>
						<Th modifier="fitContent" sort={getSortParams(2)}>
							Severity
						</Th>
						<Th sort={getSortParams(3)}>Package</Th>
						<Th modifier="fitContent">Fix Available</Th>
					</Tr>
				</Thead>
				{processedVulns.map((v) => {
					const isExpanded = expandedVulns.has(v.id);
					return (
						<Tbody key={v.id} isExpanded={isExpanded}>
							<Tr>
								<Td
									expand={{
										rowIndex: v.id,
										isExpanded,
										onToggle: () =>
											setExpandedVulns((prev) => {
												const next = new Set(prev);
												if (next.has(v.id)) {
													next.delete(v.id);
												} else {
													next.add(v.id);
												}
												return next;
											}),
									}}
								/>
								<Td>
									{v.link ? (
										<a
											href={v.link}
											target="_blank"
											rel="noopener noreferrer"
										>
											{v.name}
										</a>
									) : (
										v.name
									)}
								</Td>
								<Td>
									<Label
										color={
											severityColor[v.severity] ?? "grey"
										}
										isCompact
									>
										{v.severity || "Unknown"}
									</Label>
								</Td>
								<Td>
									<code style={{ fontSize: "0.85em" }}>
										{v.package_name || "\u2014"}
									</code>
								</Td>
								<Td>
									{v.fixed_in_version ? (
										<Label color="blue" isCompact>
											{v.fixed_in_version}
										</Label>
									) : (
										"\u2014"
									)}
								</Td>
							</Tr>
							{isExpanded && (
								<Tr isExpanded>
									<Td colSpan={5}>
										<ExpandableRowContent>
											<div
												style={{
													fontSize: "0.85em",
													whiteSpace: "pre-wrap",
													wordBreak: "break-word",
												}}
											>
												{v.description || "No description available."}
											</div>
										</ExpandableRowContent>
									</Td>
								</Tr>
							)}
						</Tbody>
					);
				})}
			</Table>
		</>
	);
}
