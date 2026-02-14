import { useEffect, useState, useCallback, useRef } from "react";

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

const cache = new Map<string, CacheEntry<unknown>>();

const DEFAULT_TTL_MS = 60_000;

export function useCachedFetch<T>(
  key: string | null,
  fetcher: () => Promise<T>,
  ttlMs = DEFAULT_TTL_MS,
): { data: T | undefined; loading: boolean; error: Error | undefined; refetch: () => void } {
  const [data, setData] = useState<T | undefined>(() => {
    if (!key) return undefined;
    const entry = cache.get(key) as CacheEntry<T> | undefined;
    return entry?.data;
  });
  const [loading, setLoading] = useState<boolean>(() => {
    if (!key) return false;
    const entry = cache.get(key);
    return !entry;
  });
  const [error, setError] = useState<Error | undefined>();
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const doFetch = useCallback(() => {
    if (!key) return;
    const entry = cache.get(key) as CacheEntry<T> | undefined;
    const now = Date.now();
    if (entry && now - entry.timestamp < ttlMs) {
      setData(entry.data);
      setLoading(false);
      return;
    }
    // Show cached data while refreshing (no loading flicker)
    if (entry) {
      setData(entry.data);
      setLoading(false);
    } else {
      setLoading(true);
    }
    fetcherRef.current()
      .then((result) => {
        cache.set(key, { data: result, timestamp: Date.now() });
        setData(result);
        setError(undefined);
      })
      .catch((err) => setError(err instanceof Error ? err : new Error(String(err))))
      .finally(() => setLoading(false));
  }, [key, ttlMs]);

  useEffect(() => {
    doFetch();
  }, [doFetch]);

  return { data, loading, error, refetch: doFetch };
}
