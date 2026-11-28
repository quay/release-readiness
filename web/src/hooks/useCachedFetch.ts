import { useCallback, useEffect, useRef, useState } from "react";

interface CacheEntry<T> {
	data: T;
	timestamp: number;
}

const cache = new Map<string, CacheEntry<unknown>>();

const DEFAULT_TTL_MS = 60_000;

export function useCachedFetch<T>(
	key: string,
	fetcher: () => Promise<T>,
	ttlMs = DEFAULT_TTL_MS,
): {
	data: T | undefined;
	loading: boolean;
	error: Error | undefined;
	refetch: () => void;
} {
	const [revision, setRevision] = useState(0);
	const [result, setResult] = useState<{
		key: string;
		data?: T;
		loading: boolean;
		error?: Error;
	}>(() => ({
		key,
		data: (cache.get(key) as CacheEntry<T> | undefined)?.data,
		loading: !cache.has(key),
	}));
	const fetcherRef = useRef(fetcher);
	fetcherRef.current = fetcher;
	const refetch = useCallback(() => {
		cache.delete(key);
		setRevision((v) => v + 1);
	}, [key]);
	useEffect(() => {
		let disposed = false,
			generation = 0;
		let pending = false;
		const load = (force = false) => {
			const entry = cache.get(key) as CacheEntry<T> | undefined;
			if (entry && !force && Date.now() - entry.timestamp < ttlMs) {
				setResult((previous) => ({
					key,
					data: entry.data,
					loading: false,
					// Keep the last error until a fetch succeeds, so a
					// failure banner does not blink away on every poll.
					error: previous.key === key ? previous.error : undefined,
				}));
				return;
			}
			// A poll or focus refresh must not discard the request still in
			// flight: a response slower than the interval would never render.
			if (pending) return;
			pending = true;
			const request = ++generation;
			setResult((previous) => ({
				key,
				data: entry?.data ?? (previous.key === key ? previous.data : undefined),
				loading: !entry,
			}));
			fetcherRef
				.current()
				.then((data) => {
					pending = false;
					if (disposed || request !== generation) return;
					cache.set(key, { data, timestamp: Date.now() });
					setResult({ key, data, loading: false });
				})
				.catch((error) => {
					pending = false;
					if (disposed || request !== generation) return;
					setResult((previous) => ({
						...previous,
						key,
						loading: false,
						error: error instanceof Error ? error : new Error(String(error)),
					}));
				});
		};
		// revision is a user-requested refresh, even inside the normal TTL.
		load(revision > 0);
		const onFocus = () => load();
		window.addEventListener("focus", onFocus);
		const timer = window.setInterval(() => load(true), Math.max(ttlMs, 1000));
		return () => {
			disposed = true;
			window.clearInterval(timer);
			window.removeEventListener("focus", onFocus);
		};
	}, [key, ttlMs, revision]);
	// Never show the previous release's evidence while a new key is loading.
	return result.key === key
		? {
				data: result.data,
				loading: result.loading,
				error: result.error,
				refetch,
			}
		: {
				data: (cache.get(key) as CacheEntry<T> | undefined)?.data,
				loading: true,
				error: undefined,
				refetch,
			};
}
