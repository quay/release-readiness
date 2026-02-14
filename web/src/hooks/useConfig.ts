import { getConfig } from "../api/client";
import type { DashboardConfig } from "../api/types";
import { useCachedFetch } from "./useCachedFetch";

const CONFIG_TTL_MS = 5 * 60_000;

export function useConfig(): DashboardConfig | undefined {
  const { data } = useCachedFetch("config", getConfig, CONFIG_TTL_MS);
  return data;
}
