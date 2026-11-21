import { readFile } from "node:fs/promises";
import { parseArgs } from "node:util";
import type { BrowserContext } from "@playwright/test";

export function reviewOptions(screenshots: boolean) {
	const { values, positionals } = parseArgs({
		allowPositionals: true,
		options: {
			width: { type: "string", multiple: true },
			route: { type: "string", multiple: true },
			help: { type: "boolean", short: "h" },
			dark: { type: "boolean" },
			theme: { type: "string", multiple: true },
		},
	});
	if ((screenshots && values.theme) || (!screenshots && values.dark))
		throw new Error(
			screenshots
				? "Use --dark for screenshots"
				: "Use --theme for accessibility",
		);
	if (values.help) {
		console.log(
			screenshots
				? "shots [--width PX]... [--dark] [--route GLOB=FILE]... BASE_URL OUT_DIR PATH..."
				: "a11y [--width PX]... [--theme light|dark]... [--route GLOB=FILE]... BASE_URL PATH...",
		);
		process.exit(0);
	}
	if (positionals.length < (screenshots ? 3 : 2))
		throw new Error("Missing arguments; use --help");
	const widths = (values.width ?? ["1440"]).map(Number);
	if (
		widths.some(
			(width) => !Number.isInteger(width) || width < 240 || width > 7680,
		)
	)
		throw new Error("Widths must be integers between 240 and 7680");
	const themes = screenshots
		? [values.dark ? "dark" : "light"]
		: (values.theme ?? ["light", "dark"]);
	if (themes.some((theme) => theme !== "light" && theme !== "dark"))
		throw new Error("Theme must be light or dark");
	const [base, ...rest] = positionals;
	if (!["http:", "https:"].includes(new URL(base).protocol))
		throw new Error("BASE_URL must be HTTP(S)");
	const output = screenshots ? rest.shift()! : undefined;
	return {
		base,
		output,
		paths: rest,
		widths,
		themes: themes as ("light" | "dark")[],
		routes: values.route ?? [],
	};
}

export async function mockRoutes(context: BrowserContext, specs: string[]) {
	for (const spec of specs) {
		const separator = spec.lastIndexOf("=");
		if (separator < 1 || separator === spec.length - 1)
			throw new Error("Route must be GLOB=FILE");
		const body = await readFile(spec.slice(separator + 1));
		JSON.parse(body.toString());
		await context.route(spec.slice(0, separator), (route) =>
			route.fulfill({ status: 200, contentType: "application/json", body }),
		);
	}
}

export function pageURL(base: string, path: string) {
	return new URL(`${base.replace(/\/$/, "")}/${path.replace(/^\//, "")}`).href;
}
