import { ObjGatewaySDK } from "obj-gateway-sdk";
import "./style.css";

const GATEWAY =
  (import.meta.env.VITE_GATEWAY_ENDPOINT as string | undefined) ||
  "http://localhost:8080/api/v1";
const TOKEN =
  (import.meta.env.VITE_GATEWAY_TOKEN as string | undefined) ||
  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjIxMDcxNzI2OTcsImlhdCI6MTc5MTgxMjY5Nywicm9sZSI6InNlcnZpY2Vfcm9sZSIsImJ1Y2tldCI6InVwbG9hZHMifQ.lMPlddcoJ4vXKj1QFun4mInrmHCAWVhyKwCPzzhiGrw";
const BUCKET = (import.meta.env.VITE_DEFAULT_BUCKET as string | undefined) || "uploads";
const MULTIPART_THRESHOLD = 8 * 1024 * 1024;

const app = document.querySelector<HTMLDivElement>("#app")!;
app.innerHTML = `
  <main class="shell">
    <header>
      <p class="eyebrow">obj-gateway</p>
      <h1>Playground</h1>
      <p class="sub">Upload via gateway presign → RustFS direct PUT. CDN at :8081.</p>
    </header>

    <section class="panel">
      <label class="field">
        <span>Object key</span>
        <input id="key" type="text" value="demo/sample.bin" />
      </label>
      <label class="field">
        <span>File</span>
        <input id="file" type="file" />
      </label>
      <div class="actions">
        <button id="upload" type="button">Upload</button>
        <button id="head" type="button" class="ghost">Head object</button>
      </div>
      <div class="progress-wrap">
        <div id="bar" class="bar"></div>
      </div>
      <p id="status" class="status">Ready · gateway ${GATEWAY}</p>
      <pre id="out" class="out"></pre>
    </section>
  </main>
`;

const storage = new ObjGatewaySDK({
  gatewayEndpoint: GATEWAY,
  token: TOKEN,
  defaultBucket: BUCKET,
});

const keyEl = document.querySelector<HTMLInputElement>("#key")!;
const fileEl = document.querySelector<HTMLInputElement>("#file")!;
const statusEl = document.querySelector<HTMLParagraphElement>("#status")!;
const outEl = document.querySelector<HTMLPreElement>("#out")!;
const barEl = document.querySelector<HTMLDivElement>("#bar")!;

function setProgress(percent: number) {
  barEl.style.width = `${Math.max(0, Math.min(100, percent))}%`;
}

document.querySelector("#upload")!.addEventListener("click", async () => {
  const file = fileEl.files?.[0];
  if (!file) {
    statusEl.textContent = "Choose a file first";
    return;
  }
  const key = keyEl.value.trim() || `demo/${file.name}`;
  statusEl.textContent = `Uploading ${file.name} (${file.size} bytes)…`;
  setProgress(0);
  outEl.textContent = "";
  try {
    const opts = {
      key,
      contentType: file.type || "application/octet-stream",
      meta: { originalname: file.name },
      onProgress: (ev: { percent: number }) => {
        setProgress(ev.percent);
        statusEl.textContent = `Uploading… ${ev.percent}%`;
      },
    };
    const res =
      file.size >= MULTIPART_THRESHOLD
        ? await storage.multipartUpload(file, opts)
        : await storage.upload(file, opts);
    setProgress(100);
    statusEl.textContent = "Upload ok";
    outEl.textContent = JSON.stringify(res, null, 2);
  } catch (err) {
    statusEl.textContent = "Upload failed";
    outEl.textContent = String(err);
  }
});

document.querySelector("#head")!.addEventListener("click", async () => {
  const key = keyEl.value.trim();
  if (!key) {
    statusEl.textContent = "Enter an object key";
    return;
  }
  statusEl.textContent = "Fetching head…";
  try {
    const head = await storage.head(key);
    statusEl.textContent = "Head ok";
    outEl.textContent = JSON.stringify(head, null, 2);
  } catch (err) {
    statusEl.textContent = "Head failed";
    outEl.textContent = String(err);
  }
});
