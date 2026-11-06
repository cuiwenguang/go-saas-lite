/* 本地预览 dist：/ -> /admin/，/admin/* -> dist/admin，/app/* -> dist/tenant，
 * SPA history fallback。生产环境此职责由 Go embed + NoRoute 承担（见 CONVENTIONS-FRONTEND §10）。 */
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../dist", import.meta.url));
const port = Number(process.env.PORT || 4173);

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
};

function send(res, code, body, type) {
  res.writeHead(code, { "content-type": type || "application/octet-stream" });
  res.end(body);
}

async function tryFile(p) {
  const safe = normalize(p);
  if (!safe.startsWith(root)) return null;
  try {
    return await readFile(safe);
  } catch {
    return null;
  }
}

createServer(async (req, res) => {
  const url = (req.url || "/").split("?")[0];

  if (url === "/" || url === "/admin" || url === "/app") {
    res.writeHead(302, { location: url === "/app" ? "/app/" : "/admin/" });
    return res.end();
  }

  if (url.startsWith("/admin/")) {
    const rest = url.slice("/admin/".length);
    const file = rest ? await tryFile(join(root, "admin", rest)) : null;
    if (file) return send(res, 200, file, MIME[extname(rest)] || "text/html");
    const index = await tryFile(join(root, "admin", "index.html"));
    if (index) return send(res, 200, index, "text/html");
  }

  if (url.startsWith("/app/")) {
    const rest = url.slice("/app/".length);
    const file = rest ? await tryFile(join(root, "tenant", rest)) : null;
    if (file) return send(res, 200, file, MIME[extname(rest)] || "text/html");
    const index = await tryFile(join(root, "tenant", "index.html"));
    if (index) return send(res, 200, index, "text/html");
  }

  // 相对引用的共享资源（/admin/../assets/* 归一化后到这里）与根级静态文件
  const norm = url.replace(/\.\./g, "").replace(/\/{2,}/g, "/");
  const file = await tryFile(join(root, norm));
  if (file) return send(res, 200, file, MIME[extname(norm)]);

  send(res, 404, "not found", "text/plain");
}).listen(port, () => {
  console.log(`preview: http://localhost:${port}/admin/  (tenant: /app/)`);
});
