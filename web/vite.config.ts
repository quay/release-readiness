import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
	// VITE_BASE bakes a URL prefix (e.g. /v2) into asset and module URLs;
	// the server must be configured with the matching -base-path.
	base: process.env.VITE_BASE || "/",
	plugins: [react()],
	build: {
		outDir: "dist",
		emptyOutDir: true,
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (
						id.includes("node_modules/react/") ||
						id.includes("node_modules/react-dom/") ||
						id.includes("node_modules/react-router-dom/") ||
						id.includes("node_modules/react-router/")
					) {
						return "react";
					}
					if (
						id.includes("node_modules/@patternfly/react-core/") ||
						id.includes("node_modules/@patternfly/react-table/") ||
						id.includes("node_modules/@patternfly/react-icons/")
					) {
						return "patternfly";
					}
				},
			},
		},
	},
	server: {
		allowedHosts: ["workstation"],
		proxy: {
			"/api": {
				target: process.env.RR_API_TARGET ?? "http://localhost:8088",
				changeOrigin: true,
			},
		},
	},
});
