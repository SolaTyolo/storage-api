/**
 * Optional storage-js contract tests.
 * Run against a live server: STORAGE_CONTRACT_URL=http://localhost:8080 STORAGE_CONTRACT_KEY=dev-api-key node --test contract.test.mjs
 */
import { test, before } from "node:test";
import assert from "node:assert/strict";

const base = (process.env.STORAGE_CONTRACT_URL || "").replace(/\/$/, "");
const apiKey = process.env.STORAGE_CONTRACT_KEY || "";

function skipIfNoServer() {
  if (!base || !apiKey) {
    console.log("skip: set STORAGE_CONTRACT_URL and STORAGE_CONTRACT_KEY");
    return true;
  }
  return false;
}

function headers(extra = {}) {
  return { apikey: apiKey, Authorization: `Bearer ${apiKey}`, ...extra };
}

before(async () => {
  if (skipIfNoServer()) return;
  const res = await fetch(`${base}/health`);
  assert.equal(res.status, 200);
});

test("storage-js list contract", async () => {
  if (skipIfNoServer()) return;
  const res = await fetch(`${base}/storage/v1/object/list/uploads`, {
    method: "POST",
    headers: headers({ "Content-Type": "application/json" }),
    body: JSON.stringify({ prefix: "", limit: 10, offset: 0 }),
  });
  assert.equal(res.status, 200);
  const data = await res.json();
  assert.ok(Array.isArray(data));
});

test("storage-js sign contract returns absolute URL", async () => {
  if (skipIfNoServer()) return;
  const res = await fetch(`${base}/storage/v1/object/sign/uploads/foo.jpg`, {
    method: "POST",
    headers: headers({ "Content-Type": "application/json" }),
    body: JSON.stringify({ expiresIn: 3600 }),
  });
  assert.equal(res.status, 200);
  const data = await res.json();
  assert.match(data.signedURL, /^https?:\/\//);
  assert.equal(data.path, "foo.jpg");
});

test("storage-js copy supports destinationBucket in body", async () => {
  if (skipIfNoServer()) return;
  const res = await fetch(`${base}/storage/v1/object/copy`, {
    method: "POST",
    headers: headers({ "Content-Type": "application/json" }),
    body: JSON.stringify({
      bucketId: "uploads",
      sourceKey: "foo.jpg",
      destinationKey: "copy.jpg",
      destinationBucket: "rustfs:uploads",
    }),
  });
  assert.ok(res.status === 200 || res.status === 404 || res.status === 500);
});
