import { defineConfig, devices } from "@playwright/test";

import { launchOptions } from "./e2e/helpers/browser.ts";

export default defineConfig({
	testDir: "./e2e",
	fullyParallel: false,
	use: {
		baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:5173",
		launchOptions,
		...devices["Desktop Chrome"],
	},
	webServer: process.env.PLAYWRIGHT_BASE_URL
		? undefined
		: {
				command: "npm run dev -- --host 127.0.0.1",
				url: "http://127.0.0.1:5173",
				reuseExistingServer: !process.env.CI,
			},
});
