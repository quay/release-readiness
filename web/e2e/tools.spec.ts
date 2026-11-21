import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { expect, test } from "@playwright/test";
import { captureFullPage, cardDiagnostics } from "./helpers/screenshots.ts";

const execute = promisify(execFile);

test("full-page helper includes PatternFly main overflow and restores viewport", async ({
	page,
}, testInfo) => {
	await page.setViewportSize({ width: 420, height: 600 });
	await page.setContent(`<style>body{margin:0}main{height:100vh;overflow:auto}.content{height:1800px}</style>
		<main class="pf-v6-c-page__main"><div class="pf-v6-c-card content">Entire card</div></main>`);
	const output = testInfo.outputPath("scrolling-main.png");
	await captureFullPage(page, output);
	const png = await readFile(output);
	expect(png.readUInt32BE(20)).toBeGreaterThanOrEqual(1800);
	expect(page.viewportSize()).toEqual({ width: 420, height: 600 });
	expect((await cardDiagnostics(page)).cards[0].text).toContain("Entire card");
});

test("CLI tools capture, audit, mock routes, and build annotated comparisons", async () => {
	test.setTimeout(60_000);
	const directory = await mkdtemp(join(tmpdir(), "rr-playwright-tools-"));
	const server = createServer((request, response) => {
		if (request.url === "/api/fixture") {
			response.setHeader("Content-Type", "application/json");
			response.end(JSON.stringify({ title: "Live title" }));
			return;
		}
		response.setHeader("Content-Type", "text/html");
		response.end(`<!doctype html><style>body{margin:0;background:white;color:black}main{height:100vh;overflow:auto}.card{min-height:1600px}h1{font:32px sans-serif}</style>
			<main class="pf-v6-c-page__main"><section class="pf-v6-c-card card"><h1>Loading</h1>
			<p style="color:${request.url === "/low" ? "#aaa" : "black"}">Measured paragraph</p></section></main>
			<script>if(matchMedia('(prefers-color-scheme:dark)').matches)document.documentElement.classList.add('pf-v6-theme-dark');
			fetch('/api/fixture').then(r=>r.json()).then(d=>document.querySelector('h1').textContent=d.title);
			${request.url === "/error" ? "console.error('Synthetic console failure')" : ""}</script>`);
	});
	try {
		await new Promise<void>((resolve) =>
			server.listen(0, "127.0.0.1", resolve),
		);
		const address = server.address();
		if (!address || typeof address === "string")
			throw new Error("No test server port");
		const base = `http://127.0.0.1:${address.port}`;
		const run = (tool: string, args: string[]) =>
			execute(process.execPath, [resolve(`e2e/tools/${tool}.ts`), ...args], {
				timeout: 20_000,
			});
		const fixture = join(directory, "fixture.json");
		await writeFile(fixture, JSON.stringify({ title: "Mocked card title" }));
		const shots = await run("shots", [
			"--width",
			"420",
			"--route",
			`**/api/fixture=${fixture}`,
			base,
			directory,
			"/",
		]);
		expect(shots.stdout).toContain("Mocked card title");
		await run("shots", ["--width", "420", "--dark", base, directory, "/"]);
		const audit = await run("a11y", ["--width", "420", base, "/"]);
		expect(audit.stdout).toContain('"failures": []');
		await expect(
			run("a11y", ["--theme", "light", base, "/low"]),
		).rejects.toMatchObject({ code: 1 });
		await expect(
			run("shots", [base, directory, "/error"]),
		).rejects.toMatchObject({ code: 1 });
		const spec = join(directory, "compare.json");
		await writeFile(
			spec,
			JSON.stringify({
				before: "overview-light-420.png",
				after: "overview-dark-420.png",
				title_before: "Before <literal>",
				title_after: "After",
				out: "pair.png",
				marks: [{ n: 1, side: "after", x: 10, y: 50, w: 60, h: 40 }],
			}),
		);
		await run("compare", [spec]);
		const png = await readFile(join(directory, "pair.png"));
		expect(png.readUInt32BE(16)).toBe(420 * 2 + 24);
		expect(png.readUInt32BE(20)).toBeGreaterThanOrEqual(1640);
		await writeFile(
			spec,
			JSON.stringify({
				sheet: [{ img: "overview-light-420.png", title: "Single" }],
				out: "sheet.png",
			}),
		);
		await run("compare", [spec]);
		expect(
			(await readFile(join(directory, "sheet.png"))).readUInt32BE(16),
		).toBe(420);
		await writeFile(
			spec,
			JSON.stringify({
				sheet: [
					{
						img: "overview-light-420.png",
						title: "Invalid",
						marks: [{ n: 1, x: 900, y: 0, w: 2, h: 2 }],
					},
				],
				out: "invalid.png",
			}),
		);
		await expect(run("compare", [spec])).rejects.toMatchObject({ code: 1 });
	} finally {
		server.closeAllConnections();
		await new Promise<void>((resolve, reject) =>
			server.close((error) => (error ? reject(error) : resolve())),
		);
		await rm(directory, { recursive: true, force: true });
	}
});
