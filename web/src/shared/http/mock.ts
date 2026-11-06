import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from "axios";
import { routes } from "../mock/routes";

/* axios 自定义适配器：命中 mock 路由表则本地返回，未命中返回 404 信封。
 * 延迟 200~500ms 模拟真实网络，让 loading 态可见。 */

function matchPattern(pattern: string, url: string): Record<string, string> | null {
  const p = pattern.split("/").filter(Boolean);
  const u = url.split("?")[0].split("/").filter(Boolean);
  if (p.length !== u.length) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < p.length; i++) {
    if (p[i].startsWith(":")) {
      params[p[i].slice(1)] = decodeURIComponent(u[i]);
    } else if (p[i] !== u[i]) {
      return null;
    }
  }
  return params;
}

export const mockAdapter: AxiosAdapter = (config: InternalAxiosRequestConfig) => {
  return new Promise<AxiosResponse>((resolve) => {
    const method = (config.method || "get").toLowerCase();
    const url = (config.url || "").replace(config.baseURL || "", "");

    setTimeout(() => {
      for (const route of routes) {
        if (route.method !== method) continue;
        const params = matchPattern(route.pattern, url);
        if (!params) continue;

        let body: unknown = undefined;
        if (typeof config.data === "string" && config.data) {
          try { body = JSON.parse(config.data); } catch { body = {}; }
        } else if (config.data && typeof config.data === "object") {
          body = config.data;
        }

        const query: Record<string, string> = {};
        const qp = (config.params || {}) as Record<string, unknown>;
        for (const [k, v] of Object.entries(qp)) {
          if (v !== undefined && v !== null && v !== "") query[k] = String(v);
        }

        const result = route.handler({
          params,
          query,
          body: (body || {}) as Record<string, unknown>,
          headers: config.headers || {},
        });

        /* 只有数字 code 才是统一信封：业务对象自身也可能有 string 类型的 code 字段（如租户标识） */
        const isEnvelope = !!result
          && typeof result === "object"
          && typeof (result as { code?: unknown }).code === "number";

        const env = isEnvelope
          ? result as { code: number; msg?: string; data?: unknown }
          : { code: 0, msg: "ok", data: result };

        resolve({
          data: { code: env.code, msg: env.msg ?? "ok", data: env.data ?? null },
          status: 200,
          statusText: "OK",
          headers: {},
          config,
        });
        return;
      }

      resolve({
        data: { code: 404, msg: `mock 未命中: ${method.toUpperCase()} ${url}`, data: null },
        status: 200,
        statusText: "OK",
        headers: {},
        config,
      });
    }, 200 + Math.random() * 300);
  });
};
