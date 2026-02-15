import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [react()],
	build: {
		outDir: "dist",
		emptyOutDir: true,
		rollupOptions: {
			output: {
				manualChunks: {
					react: ["react", "react-dom", "react-router-dom"],
					patternfly: [
						"@patternfly/react-core",
						"@patternfly/react-table",
						"@patternfly/react-icons",
					],
				},
			},
		},
	},
	server: {
		allowedHosts: ["workstation"],
		proxy: {
			"/api": {
				target: "http://localhost:8088",
				changeOrigin: true,
			},
		},
	},
});
