export function formatDuration(seconds: number): string {
	if (seconds < 60) return `${seconds.toFixed(1)}s`;
	const m = Math.floor(seconds / 60);
	const s = Math.round(seconds % 60);
	return `${m}m ${s}s`;
}
