import {
	Flex,
	FlexItem,
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
import type { TestCase } from "../api/types";
import StatusLabel from "./StatusLabel";

const statusWeight: Record<string, number> = {
	failed: 0,
	error: 1,
	pending: 2,
	skipped: 3,
	passed: 4,
};

export default function TestCasesTable({
	testCases,
}: {
	testCases: TestCase[];
}) {
	const [activeSortIndex, setActiveSortIndex] = useState<number | undefined>(
		undefined,
	);
	const [activeSortDirection, setActiveSortDirection] = useState<
		"asc" | "desc" | undefined
	>(undefined);
	const [nameFilter, setNameFilter] = useState("");
	const [statusFilters, setStatusFilters] = useState<Set<string>>(new Set());
	const [expandedCases, setExpandedCases] = useState<Set<number>>(new Set());

	const availableStatuses = useMemo(() => {
		const statuses = new Set<string>();
		for (const tc of testCases) {
			statuses.add(tc.status.toLowerCase());
		}
		return [...statuses].sort(
			(a, b) => (statusWeight[a] ?? 99) - (statusWeight[b] ?? 99),
		);
	}, [testCases]);

	const processedCases = useMemo(() => {
		let cases = [...testCases];

		if (nameFilter) {
			const lower = nameFilter.toLowerCase();
			cases = cases.filter((tc) => tc.name.toLowerCase().includes(lower));
		}
		if (statusFilters.size > 0) {
			cases = cases.filter((tc) => statusFilters.has(tc.status.toLowerCase()));
		}

		if (activeSortIndex !== undefined && activeSortDirection !== undefined) {
			cases.sort((a, b) => {
				let cmp = 0;
				switch (activeSortIndex) {
					case 1:
						cmp = a.name.localeCompare(b.name);
						break;
					case 2:
						cmp =
							(statusWeight[a.status.toLowerCase()] ?? 99) -
							(statusWeight[b.status.toLowerCase()] ?? 99);
						break;
				}
				return activeSortDirection === "asc" ? cmp : -cmp;
			});
		}

		return cases;
	}, [
		testCases,
		nameFilter,
		statusFilters,
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
							placeholder="Filter by test name..."
							value={nameFilter}
							onChange={(_e, val) => setNameFilter(val)}
							onClear={() => setNameFilter("")}
						/>
					</ToolbarItem>
					<ToolbarItem>
						<ToggleGroup aria-label="Status filter">
							{availableStatuses.map((s) => (
								<ToggleGroupItem
									key={s}
									text={s.charAt(0).toUpperCase() + s.slice(1)}
									isSelected={statusFilters.has(s)}
									onChange={() => {
										setStatusFilters((prev) => {
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
			{(nameFilter || statusFilters.size > 0) && (
				<div
					style={{
						fontSize: "0.85em",
						padding: "0.25rem 0",
						color: "var(--pf-t--global--text--color--subtle)",
					}}
				>
					{processedCases.length} of {testCases.length} test cases
				</div>
			)}
			<Table variant="compact" borders={false}>
				<Thead>
					<Tr>
						<Th screenReaderText="Toggle" />
						<Th sort={getSortParams(1)}>Test Name</Th>
						<Th modifier="fitContent" sort={getSortParams(2)}>
							Status
						</Th>
						<Th modifier="fitContent">Duration</Th>
						<Th>File</Th>
					</Tr>
				</Thead>
				{processedCases.map((tc) => {
					const isCaseExpanded = expandedCases.has(tc.id);
					const hasDetails = !!(tc.message || tc.trace);
					return (
						<Tbody key={tc.id} isExpanded={isCaseExpanded}>
							<Tr>
								<Td
									expand={
										hasDetails
											? {
													rowIndex: tc.id,
													isExpanded: isCaseExpanded,
													onToggle: () =>
														setExpandedCases((prev) => {
															const next = new Set(prev);
															if (next.has(tc.id)) {
																next.delete(tc.id);
															} else {
																next.add(tc.id);
															}
															return next;
														}),
												}
											: undefined
									}
								/>
								<Td>{tc.name}</Td>
								<Td>
									<StatusLabel status={tc.status} />
								</Td>
								<Td>
									{tc.duration_ms > 0
										? `${(tc.duration_ms / 1000).toFixed(1)}s`
										: "\u2014"}
								</Td>
								<Td>
									<code style={{ fontSize: "0.85em" }}>
										{tc.file_path || "\u2014"}
									</code>
								</Td>
							</Tr>
							{isCaseExpanded && (
								<Tr isExpanded>
									<Td colSpan={5}>
										<ExpandableRowContent>
											<Flex>
												<FlexItem flex={{ default: "flex_1" }}>
													<div className="rr-label">Message</div>
													<pre
														style={{
															whiteSpace: "pre-wrap",
															wordBreak: "break-word",
															fontSize: "0.85em",
															margin: 0,
														}}
													>
														{tc.message || "\u2014"}
													</pre>
												</FlexItem>
												<FlexItem flex={{ default: "flex_1" }}>
													<div className="rr-label">Trace</div>
													<pre
														style={{
															whiteSpace: "pre-wrap",
															wordBreak: "break-word",
															fontSize: "0.85em",
															margin: 0,
														}}
													>
														{tc.trace || "\u2014"}
													</pre>
												</FlexItem>
											</Flex>
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
