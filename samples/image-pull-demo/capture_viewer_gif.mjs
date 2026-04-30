#!/usr/bin/env node
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, "../..");
const frameDir = "/tmp/kubernetes-ontology-viewer-gif-frames";
const width = Number(process.env.CAPTURE_WIDTH || "1600");
const height = Number(process.env.CAPTURE_HEIGHT || "900");
const chromeBin = process.env.CHROME_BIN || detectChrome();
const locale = normalizeLocale(process.env.CAPTURE_LOCALE || "zh-CN");
const scenarioName = process.env.CAPTURE_SCENARIO || "image-pull-demo";
const scenario = scenarioConfig(scenarioName, locale);
const graphPath = process.env.CAPTURE_GRAPH || scenario.graphPath;
const outputPath = resolve(repoRoot, process.env.CAPTURE_OUTPUT || scenario.output);

if (!chromeBin) {
  throw new Error("Chrome was not found. Set CHROME_BIN to a Chrome or Chromium executable.");
}

function normalizeLocale(value) {
  const normalized = value.toLowerCase();
  if (normalized === "en" || normalized === "en-us") return "en";
  if (normalized === "zh" || normalized === "zh-cn" || normalized === "zh-hans") return "zh-CN";
  throw new Error(`unsupported CAPTURE_LOCALE: ${value}`);
}

function scenarioConfig(name, value) {
  const configs = {
    "image-pull-demo": {
      graphPath: "samples/image-pull-demo/diagnostic-graph.json",
      locales: {
        en: {
          output: "docs/assets/image-pull-demo.en.gif",
          notes: [
        {
          title: "Full Diagnostic Topology",
          body: "Starting from one ImagePullBackOff Pod, the viewer shows owner, Service, config, identity/RBAC, image, and Event evidence in one graph.",
          slug: "real-viewer-loaded",
        },
        {
          title: "Entry Node: Failing Pod",
          body: "Selecting the Pod shows phase and reason in the details panel while the graph keeps the full troubleshooting context visible.",
          parts: ["Pod", "ontology-demo", "checkout-api"],
          slug: "select-pod",
        },
        {
          title: "Owner Chain Explains Blast Radius",
          body: "Deployment, ReplicaSet, and Pod ownership shows which workload owns the failure and helps judge rollout or rollback impact.",
          parts: ["Workload", "ontology-demo", "checkout-api"],
          slug: "select-workload",
        },
        {
          title: "Image Dependency Points to a Root-Cause Candidate",
          body: "The uses_image relation connects the Pod to the actual image tag; the missing tag is the key ImagePullBackOff clue.",
          parts: ["Image", "missing-tag"],
          slug: "select-image",
        },
        {
          title: "Events Provide Failure Evidence",
          body: "Event nodes preserve kubelet pull and BackOff evidence, helping humans and agents converge from topology to concrete error.",
          parts: ["Event", "ontology-demo", "checkout-api"],
          slug: "select-event",
        },
        {
          title: "Identity and RBAC Stay in Context",
          body: "The same graph includes ServiceAccount and RoleBinding evidence, so identity or permission context is not lost during diagnosis.",
          parts: ["ServiceAccount", "ontology-demo"],
          slug: "select-identity",
        },
          ],
        },
        "zh-CN": {
          output: "docs/assets/image-pull-demo.gif",
          notes: [
        {
          title: "完整诊断拓扑",
          body: "从一个 ImagePullBackOff Pod 出发，viewer 一次性展示 owner、Service、配置、身份/RBAC、镜像和 Event 证据。",
          slug: "real-viewer-loaded",
        },
        {
          title: "入口节点：故障 Pod",
          body: "选中 Pod 后，右侧详情能看到 phase/reason；中间图保留完整上下文，不会只剩一段局部链路。",
          parts: ["Pod", "ontology-demo", "checkout-api"],
          slug: "select-pod",
        },
        {
          title: "owner 链路解释影响范围",
          body: "Deployment、ReplicaSet、Pod 的 owner 关系说明故障属于哪个 workload，便于判断发布或回滚影响面。",
          parts: ["Workload", "ontology-demo", "checkout-api"],
          slug: "select-workload",
        },
        {
          title: "镜像依赖指向根因候选",
          body: "uses_image 关系把 Pod 和实际镜像 tag 连起来；这里的 missing-tag 是 ImagePullBackOff 的关键线索。",
          parts: ["Image", "missing-tag"],
          slug: "select-image",
        },
        {
          title: "Event 提供失败证据",
          body: "Event 节点保留 kubelet 报错和 BackOff 证据，让人和 Agent 都能把根因从拓扑关系收敛到具体错误。",
          parts: ["Event", "ontology-demo", "checkout-api"],
          slug: "select-event",
        },
        {
          title: "身份和 RBAC 不丢上下文",
          body: "同一张图也带出 ServiceAccount 与 RoleBinding，排除镜像问题之外的身份/权限干扰因素。",
          parts: ["ServiceAccount", "ontology-demo"],
          slug: "select-identity",
        },
          ],
        },
      },
    },
    "kind-helm-storage": {
      graphPath: "samples/kind-helm-storage-demo/diagnostic-graph.json",
      locales: {
        en: {
          output: "docs/assets/kind-helm-storage-demo.en.gif",
          notes: [
        {
          title: "Kind Helm Storage Topology",
          body: "A Helm-installed checkout workload is shown with Service, config, identity, PVC, PV, StorageClass, CSIDriver, local-path provisioner, Node, and Event evidence.",
          slug: "real-viewer-loaded",
        },
        {
          title: "Helm Release Owns Runtime Objects",
          body: "The chart and release sit beside the Kubernetes objects they produced, so deployment provenance stays visible during diagnosis.",
          parts: ["HelmRelease", "payments", "checkout"],
          slug: "select-helm-release",
        },
        {
          title: "Workload Path To The Pod",
          body: "Deployment and ReplicaSet ownership explain which rollout produced the Pod and how far a rollback would reach.",
          parts: ["Pod", "payments", "checkout-api"],
          slug: "select-pod",
        },
        {
          title: "PVCs Mark The Storage Boundary",
          body: "The consuming Pod mounts data and cache claims, making the application-to-storage dependency explicit instead of buried in YAML.",
          parts: ["PVC", "payments", "checkout-data"],
          slug: "select-pvc",
        },
        {
          title: "PV And StorageClass Explain Binding",
          body: "The bound PVs carry node affinity and point to the standard StorageClass, which is the handoff into local-path provisioning.",
          parts: ["StorageClass", "standard"],
          slug: "select-storageclass",
        },
        {
          title: "CSI Driver Connects Storage To Control Plane",
          body: "The StorageClass provisioner resolves to the CSIDriver and its controller implementation, closing the loop between claims and cluster storage plumbing.",
          parts: ["CSIDriver", "rancher.io/local-path"],
          slug: "select-csidriver",
        },
        {
          title: "Provisioner Pod And Events Keep Evidence Nearby",
          body: "The graph keeps local-path provisioner placement and mount/provisioning Events in the same visual context as the consuming workload.",
          parts: ["Workload", "local-path-storage", "local-path-provisioner"],
          slug: "select-provisioner",
        },
          ],
        },
        "zh-CN": {
          output: "docs/assets/kind-helm-storage-demo.gif",
          notes: [
        {
          title: "kind Helm 存储拓扑",
          body: "Helm 安装的 checkout workload 与 Service、配置、身份、PVC、PV、StorageClass、CSIDriver、local-path provisioner、Node 和 Event 证据同屏呈现。",
          slug: "real-viewer-loaded",
        },
        {
          title: "Helm Release 连接运行时对象",
          body: "Chart 与 release 放在它产出的 Kubernetes 对象旁边，排障时不会丢掉部署来源。",
          parts: ["HelmRelease", "payments", "checkout"],
          slug: "select-helm-release",
        },
        {
          title: "workload 链路追到 Pod",
          body: "Deployment、ReplicaSet、Pod 的 owner 关系说明这个 Pod 来自哪次发布，也能判断回滚影响面。",
          parts: ["Pod", "payments", "checkout-api"],
          slug: "select-pod",
        },
        {
          title: "PVC 标出存储边界",
          body: "Pod 同时挂载 data 和 cache 两个 claim，应用到存储的依赖不再藏在 YAML 里。",
          parts: ["PVC", "payments", "checkout-data"],
          slug: "select-pvc",
        },
        {
          title: "PV 与 StorageClass 解释绑定",
          body: "已绑定的 PV 带着 node affinity，并指向 standard StorageClass，这是进入 local-path provisioner 的关键交接点。",
          parts: ["StorageClass", "standard"],
          slug: "select-storageclass",
        },
        {
          title: "CSI Driver 串起存储控制面",
          body: "StorageClass 的 provisioner 解析到 CSIDriver 和对应 controller，实现从 claim 到集群存储组件的闭环。",
          parts: ["CSIDriver", "rancher.io/local-path"],
          slug: "select-csidriver",
        },
        {
          title: "provisioner 与 Event 保留证据",
          body: "local-path provisioner 的位置、挂载与 provision Event 都保留在同一张图里，方便从应用一路追到存储实现。",
          parts: ["Workload", "local-path-storage", "local-path-provisioner"],
          slug: "select-provisioner",
        },
          ],
        },
      },
    },
  };
  const config = configs[name];
  if (!config) {
    throw new Error(`unsupported CAPTURE_SCENARIO: ${name}`);
  }
  const localized = config.locales[value];
  if (!localized) {
    throw new Error(`unsupported locale ${value} for CAPTURE_SCENARIO=${name}`);
  }
  return { graphPath: config.graphPath, ...localized };
}

function detectChrome() {
  const candidates = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "google-chrome",
    "chromium",
    "chrome",
  ];
  return candidates.find((candidate) => candidate.includes("/") ? existsSync(candidate) : true);
}

function freePort() {
  return new Promise((resolvePort, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => resolvePort(port));
    });
    server.on("error", reject);
  });
}

function wait(ms) {
  return new Promise((resolveWait) => setTimeout(resolveWait, ms));
}

async function waitForHTTP(url, timeoutMs = 8000) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) return;
      lastError = new Error(`${url} returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await wait(150);
  }
  throw lastError || new Error(`timed out waiting for ${url}`);
}

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.id = 1;
    this.pending = new Map();
    this.events = new Map();
    ws.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.id && this.pending.has(message.id)) {
        const { resolveCall, rejectCall } = this.pending.get(message.id);
        this.pending.delete(message.id);
        if (message.error) rejectCall(new Error(message.error.message));
        else resolveCall(message.result || {});
        return;
      }
      const listeners = this.events.get(message.method) || [];
      for (const listener of listeners) listener(message.params || {});
    });
  }

  static connect(url) {
    return new Promise((resolveConnect, reject) => {
      const ws = new WebSocket(url);
      ws.addEventListener("open", () => resolveConnect(new CDP(ws)));
      ws.addEventListener("error", reject);
    });
  }

  call(method, params = {}) {
    const id = this.id++;
    this.ws.send(JSON.stringify({ id, method, params }));
    return new Promise((resolveCall, rejectCall) => {
      this.pending.set(id, { resolveCall, rejectCall });
    });
  }

  waitEvent(method, timeoutMs = 8000) {
    return new Promise((resolveEvent, reject) => {
      const timer = setTimeout(() => reject(new Error(`timed out waiting for ${method}`)), timeoutMs);
      const listener = (params) => {
        clearTimeout(timer);
        this.events.set(method, (this.events.get(method) || []).filter((item) => item !== listener));
        resolveEvent(params);
      };
      this.events.set(method, [...(this.events.get(method) || []), listener]);
    });
  }

  close() {
    this.ws.close();
  }
}

async function evaluate(page, expression) {
  const result = await page.call("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.text || "Runtime.evaluate failed");
  }
  return result.result?.value;
}

async function waitForGraph(page) {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const count = await evaluate(page, `
      window.__KO_VIEWER__ ? window.__KO_VIEWER__.nodeCount() : document.querySelectorAll('.node-group').length
    `);
    if (count > 0) return;
    await wait(200);
  }
  throw new Error("viewer did not render graph nodes");
}

async function capture(page, name) {
  const path = `${frameDir}/${name}.png`;
  const screenshot = await page.call("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: false,
  });
  writeFileSync(path, Buffer.from(screenshot.data, "base64"));
  return path;
}

async function convertFramesToGIF(framePaths) {
  const code = `
import sys
from pathlib import Path
from PIL import Image

out = Path(sys.argv[1])
frames = [Image.open(path).convert("RGB") for path in sys.argv[2:]]
durations = [2100] + ([2300] * max(0, len(frames) - 2)) + ([2800] if len(frames) > 1 else [])
out.parent.mkdir(parents=True, exist_ok=True)
frames[0].save(out, save_all=True, append_images=frames[1:], duration=durations[:len(frames)], loop=0, optimize=True)
print(out)
`;
  await new Promise((resolveConvert, reject) => {
    const child = spawn("python3", ["-c", code, outputPath, ...framePaths], {
      cwd: repoRoot,
      stdio: "inherit",
    });
    child.on("exit", (codeValue) => {
      if (codeValue === 0) resolveConvert();
      else reject(new Error(`python3 exited with ${codeValue}`));
    });
  });
}

function startViewer(port) {
  return spawn("python3", ["tools/visualize/server.py", "--host", "127.0.0.1", "--port", String(port)], {
    cwd: repoRoot,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function startChrome(debugPort, userDataDir) {
  return spawn(chromeBin, [
    "--headless=new",
    "--disable-gpu",
    `--remote-debugging-port=${debugPort}`,
    `--user-data-dir=${userDataDir}`,
    `--window-size=${width},${height}`,
    "about:blank",
  ], {
    cwd: repoRoot,
    stdio: ["ignore", "ignore", "pipe"],
  });
}

async function main() {
  rmSync(frameDir, { recursive: true, force: true });
  mkdirSync(frameDir, { recursive: true });

  const viewerPort = await freePort();
  const debugPort = await freePort();
  const viewer = startViewer(viewerPort);
  const chrome = startChrome(debugPort, `/tmp/kubernetes-ontology-chrome-${Date.now()}`);

  try {
    await waitForHTTP(`http://127.0.0.1:${viewerPort}/`);
    await waitForHTTP(`http://127.0.0.1:${debugPort}/json/version`);

    const version = await (await fetch(`http://127.0.0.1:${debugPort}/json/version`)).json();
    const browser = await CDP.connect(version.webSocketDebuggerUrl);
    const { targetId } = await browser.call("Target.createTarget", { url: "about:blank" });
    const targets = await (await fetch(`http://127.0.0.1:${debugPort}/json/list`)).json();
    const target = targets.find((item) => item.id === targetId);
    const page = await CDP.connect(target.webSocketDebuggerUrl);

    await page.call("Page.enable");
    await page.call("Runtime.enable");
    await page.call("Emulation.setDeviceMetricsOverride", {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: false,
    });

    const url = `http://127.0.0.1:${viewerPort}/?file=${encodeURIComponent(graphPath)}`;
    const load = page.waitEvent("Page.loadEventFired");
    await page.call("Page.navigate", { url });
    await load;
    await waitForGraph(page);

    await evaluate(page, `
      (() => {
        document.body.classList.add('capture-mode');
        const style = document.createElement('style');
        style.textContent = [
          '.capture-mode .canvas-state{display:none!important}',
          '.capture-mode .node-label,.capture-mode .node-kind-label,.capture-mode .edge-label{font-weight:700}',
          '.capture-note{position:absolute;left:18px;right:18px;bottom:18px;z-index:12;padding:13px 15px;border:1px solid rgba(56,189,248,.45);border-radius:8px;background:rgba(17,21,29,.94);box-shadow:0 18px 50px rgba(0,0,0,.28);font-size:14px;line-height:1.45;color:#e5e7eb}',
          '.capture-note strong{display:block;margin-bottom:3px;color:#bae6fd;font-size:15px}',
          '.capture-mode .toolbar{background:rgba(17,21,29,.84)}'
        ].join('');
        document.head.appendChild(style);
        const note = document.createElement('div');
        note.id = 'captureNote';
        note.className = 'capture-note';
        document.querySelector('.canvas-wrap').appendChild(note);
        window.__KO_VIEWER__?.setEdgeLabels(true);
      })()
    `);
    await wait(400);

    const fitGraph = `
      (() => {
        if (window.__KO_VIEWER__) {
          window.__KO_VIEWER__.fit();
          return;
        }
        const svg = document.getElementById('graph');
        const nodes = document.getElementById('nodesLayer');
        const viewport = document.getElementById('graphViewport');
        const circles = [...nodes.querySelectorAll('circle')].map((circle) => ({
          x: Number(circle.getAttribute('cx')),
          y: Number(circle.getAttribute('cy')),
          r: Number(circle.getAttribute('r') || 0)
        }));
        if (!circles.length) return;
        const margin = 95;
        const minX = Math.min(...circles.map((circle) => circle.x - circle.r)) - margin;
        const maxX = Math.max(...circles.map((circle) => circle.x + circle.r)) + margin * 2.2;
        const minY = Math.min(...circles.map((circle) => circle.y - circle.r)) - margin;
        const maxY = Math.max(...circles.map((circle) => circle.y + circle.r)) + margin;
        const bbox = { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
        if (!bbox.width || !bbox.height) return;
        const canvasWidth = svg.clientWidth;
        const canvasHeight = svg.clientHeight;
        const target = {
          x: 38,
          y: 130,
          width: canvasWidth - 76,
          height: canvasHeight - 255
        };
        const fitScale = Math.min(target.width / bbox.width, target.height / bbox.height);
        const scale = Math.max(0.1, Math.min(4, fitScale * 1.85));
        const panX = target.x + (target.width - bbox.width * scale) / 2 - bbox.x * scale + 110;
        const panY = target.y + (target.height - bbox.height * scale) / 2 - bbox.y * scale;
        viewport.setAttribute('transform', 'translate(' + panX + ' ' + panY + ') scale(' + scale + ')');
      })()
    `;
    const setNote = (title, body) => evaluate(page, `
      (() => {
        const note = document.getElementById('captureNote');
        note.innerHTML = '<strong>${escapeForJS(title)}</strong>${escapeForJS(body)}';
      })()
    `);
    const selectNode = (parts, errorMessage) => evaluate(page, `
      (() => {
        if (window.__KO_VIEWER__) {
          return window.__KO_VIEWER__.selectNodeByText(${JSON.stringify(parts)});
        }
        const required = ${JSON.stringify(parts)}.map((part) => String(part).toLowerCase());
        const group = [...document.querySelectorAll('.node-group')].find((item) => {
          const text = item.textContent.toLowerCase();
          return required.every((part) => text.includes(part));
        });
        if (!group) throw new Error('${escapeForJS(errorMessage)}');
        group.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }));
      })()
    `);

    await evaluate(page, fitGraph);
    await wait(300);

    const frames = [];
    const notes = scenario.notes;
    await setNote(notes[0].title, notes[0].body);
    frames.push(await capture(page, `01-${notes[0].slug || "real-viewer-loaded"}`));

    for (const [index, note] of notes.slice(1).entries()) {
      if (note.parts) {
        await selectNode(note.parts, `${note.title} node not found`);
        await wait(450);
        await evaluate(page, fitGraph);
      }
      await setNote(note.title, note.body);
      const frameIndex = String(index + 2).padStart(2, "0");
      frames.push(await capture(page, `${frameIndex}-${note.slug || "step"}`));
    }

    await convertFramesToGIF(frames);
    console.log(`wrote ${outputPath}`);

    page.close();
    browser.close();
  } finally {
    chrome.kill("SIGTERM");
    viewer.kill("SIGTERM");
  }
}

function escapeForJS(value) {
  return String(value)
    .replaceAll("\\\\", "\\\\\\\\")
    .replaceAll("'", "\\\\'")
    .replaceAll("\n", "\\\\n");
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
