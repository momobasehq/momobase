import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: { "@": path.resolve(import.meta.dirname, "src") },
	},
	// The Go binary embeds dist/ and serves it under /dashboard/, so every asset URL
	// has to be written with that prefix at build time.
	base: "/dashboard/",
	build: {
		// Sourcemaps would add megabytes to the binary for no operator benefit, and the
		// manifest is only useful to a server that rewrites asset URLs — Go serves the
		// built index.html as-is.
		sourcemap: false,
		manifest: false,
	},
});
