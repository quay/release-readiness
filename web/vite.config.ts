import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [react()],
	build: {
		outDir: "dist",
		emptyOutDir: true,
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
