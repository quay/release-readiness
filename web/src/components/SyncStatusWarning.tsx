import {
	Button,
	Content,
	Flex,
	Icon,
	Popover,
	Stack,
	StackItem,
	Tooltip,
} from "@patternfly/react-core";
import { ExclamationTriangleIcon, SyncAltIcon } from "@patternfly/react-icons";
import { useCallback, useEffect, useRef, useState } from "react";
import { getSyncStatus } from "../api/client";
import type { SyncProblem } from "../api/types";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { relative } from "../utils/format";
import { freshnessLine, SOURCE_NAMES } from "../utils/releaseDetail";

const POLL_MS = 60_000;

/** The sync status, polled; every caller shares the one cache entry. */
export function useSyncStatus() {
	// useCachedFetch owns polling; adding another timer duplicates requests.
	const { data } = useCachedFetch("sync-status", getSyncStatus, POLL_MS);
	return data;
}

// What each sync feeds, so a problem says what it may have left out of date.
const AFFECTS: Record<string, string> = {
	konflux: "snapshots, releases, stage/prod status",
	"art-builds": "builds",
	"art-history": "Current-release builds",
	"art-backfill": "Historical ART builds",
	"fbc-render": "FBC bundles",
	catalog: "shipped versions",
	jira: "tickets",
	"image-scans": "CVE scans",
	prow: "Current-release CI runs",
	"prow-history": "Historical CI evidence",
	"prow-inventory": "Configured CI jobs",
	"shipment-mrs": "shipment MRs",
};

/**
 * Each problem: its message, since when it fails, its last success, and what
 * it affects.
 */
export function SyncProblems({ problems }: { problems: SyncProblem[] }) {
	return (
		<Stack hasGutter>
			{problems.map((p) => (
				<StackItem key={p.source}>
					<Content>
						<Content component="p">
							<strong>{SOURCE_NAMES[p.source] ?? p.source}</strong>: {p.message}
						</Content>
						<Content component="small">
							{p.since && `since ${relative(p.since)} · `}
							last success {p.last_success ? relative(p.last_success) : "never"}
						</Content>
						{AFFECTS[p.source] && (
							<Content component="p">Affects: {AFFECTS[p.source]}</Content>
						)}
						{/* Failing sources carry since; stale ones do not. */}
						{!p.since && (
							<Content component="p">
								Builds or snapshots from the stale period may not be visible
								yet.
							</Content>
						)}
					</Content>
				</StackItem>
			))}
		</Stack>
	);
}

const exact = (iso: string) =>
	new Date(iso).toLocaleString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "2-digit",
		minute: "2-digit",
		timeZoneName: "short",
	});

/**
 * The masthead's sync state on every page: a refresh icon button beside the
 * other masthead icons whose hover and name say "Updated N ago" (the exact
 * time on a second line), after "N sources stale", which opens the problems.
 * Compact, at phone width, the one icon button also names the problems and
 * opens them on click.
 */
export default function SyncStatusWarning({ compact }: { compact?: boolean }) {
	const status = useSyncStatus();
	const iconRef = useRef<HTMLButtonElement>(null);
	const warningRef = useRef<HTMLButtonElement>(null);
	const [placement, setPlacement] = useState<"bottom-start" | "bottom-end">(
		"bottom-end",
	);
	const [popoverWidth, setPopoverWidth] = useState(
		"min(36rem, calc(100vw - 2rem))",
	);
	const fitPopover = useCallback(() => {
		const trigger = compact ? iconRef.current : warningRef.current;
		if (!trigger) return;
		const rect = trigger.getBoundingClientRect();
		const leftSpace = rect.right - 16;
		const rightSpace = window.innerWidth - rect.left - 16;
		setPlacement(leftSpace >= rightSpace ? "bottom-end" : "bottom-start");
		setPopoverWidth(`min(36rem, ${Math.max(leftSpace, rightSpace)}px)`);
	}, [compact]);
	useEffect(() => {
		window.addEventListener("resize", fitPopover);
		return () => window.removeEventListener("resize", fitPopover);
	}, [fitPopover]);
	if (!status) return null;
	const line = freshnessLine(status);
	const n = line.problems.length;
	const stale = n > 0 ? `${n} source${n === 1 ? "" : "s"} stale` : null;
	const words = [line.updated, stale].filter(Boolean).join(" · ");
	if (!words) return null;
	const warning = (
		<Icon status="warning" isInline={!compact}>
			<ExclamationTriangleIcon />
		</Icon>
	);
	// The Tooltip takes the button by ref: wrapped in it, the compact Popover
	// anchors on the Tooltip's display: contents box and opens at the corner.
	const icon = (label: string, stateIcon: React.ReactNode) => (
		<>
			<Button
				ref={iconRef}
				variant="plain"
				aria-label={label}
				icon={stateIcon}
			/>
			<Tooltip
				triggerRef={iconRef}
				content={
					<>
						<div>{label}</div>
						{line.oldest && <div>{exact(line.oldest)}</div>}
					</>
				}
			/>
		</>
	);

	if (compact) {
		return (
			<Popover
				position={placement}
				enableFlip={false}
				maxWidth={popoverWidth}
				onShow={fitPopover}
				className="rr-source-popover"
				triggerRef={iconRef}
				headerContent={words}
				bodyContent={
					<Stack hasGutter>
						{line.oldest && <StackItem>{exact(line.oldest)}</StackItem>}
						{n > 0 && (
							<StackItem>
								<SyncProblems problems={line.problems} />
							</StackItem>
						)}
					</Stack>
				}
			>
				{icon(words, n > 0 ? warning : <SyncAltIcon />)}
			</Popover>
		);
	}

	return (
		<Flex
			gap={{ default: "gapMd" }}
			alignItems={{ default: "alignItemsCenter" }}
			flexWrap={{ default: "nowrap" }}
		>
			{stale && (
				<Popover
					position={placement}
					enableFlip={false}
					maxWidth={popoverWidth}
					onShow={fitPopover}
					className="rr-source-popover"
					headerContent="Sync problems"
					bodyContent={<SyncProblems problems={line.problems} />}
				>
					<Button
						ref={warningRef}
						variant="link"
						isInline
						size="sm"
						icon={warning}
						style={{ fontSize: "var(--pf-t--global--font--size--sm)" }}
					>
						{stale}
					</Button>
				</Popover>
			)}
			{line.updated && icon(line.updated, <SyncAltIcon />)}
		</Flex>
	);
}
