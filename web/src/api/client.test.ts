import assert from "node:assert/strict";
import { test } from "node:test";
import { withBase } from "./client.ts";

test("withBase joins any path onto any base form", () => {
	assert.equal(withBase("/", "api/v1"), "/api/v1");
	assert.equal(withBase("/v2", "api/v1"), "/v2/api/v1");
	assert.equal(withBase("/v2/", "api/v1"), "/v2/api/v1");
	assert.equal(withBase("/v2", "favicon.png"), "/v2/favicon.png");
	assert.equal(withBase("/v2", "/favicon.png"), "/v2/favicon.png");
});
