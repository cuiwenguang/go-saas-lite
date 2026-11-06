/* 请求唯一通道（CONVENTIONS-FRONTEND §4）
 * - 所有请求必须经由此实例；URL 只写在 shared/api/*
 * - 响应信封 {code,msg,data} 统一解包；code != 0 抛 AppError
 * - 401 对业务透明：回调注入，由 main 负责清态并跳登录
 *   （真实后端接入后在此挂「挂起 -> refresh -> 重放」队列）
 */
import axios, { AxiosError, type AxiosInstance, type InternalAxiosRequestConfig } from "axios";
import { mockAdapter } from "./mock";
import { progressDone, progressStart } from "@/shared/utils/progress";

export class AppError extends Error {
  readonly code: number;
  constructor(code: number, msg: string) {
    super(msg);
    this.name = "AppError";
    this.code = code;
  }
}

export const http: AxiosInstance = axios.create({ timeout: 15000 });

if (import.meta.env.VITE_USE_MOCK !== "false") {
  http.defaults.adapter = mockAdapter;
}

export let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

/* token 注入：由 main 入口接线（避免 client 反向依赖 auth store 造成循环 import） */
let tokenProvider: (() => string) | null = null;
export function setTokenProvider(fn: () => string) {
  tokenProvider = fn;
}

/* 401 透明刷新：挂起中的请求共用一次刷新，成功后重放原请求；
 * 刷新失败才交给 onUnauthorized（清登录态并跳登录页）。 */
type RetryConfig = InternalAxiosRequestConfig & { _retried?: boolean };

let refreshHandler: (() => Promise<string>) | null = null;
export function setRefreshHandler(fn: () => Promise<string>) {
  refreshHandler = fn;
}

let refreshing: Promise<string> | null = null;
function refreshAccessToken(): Promise<string> {
  if (!refreshHandler) return Promise.reject(new Error("refresh handler not configured"));
  if (!refreshing) {
    refreshing = refreshHandler().finally(() => {
      refreshing = null;
    });
  }
  return refreshing;
}

async function replayAfterRefresh(config: RetryConfig) {
  config._retried = true;
  const token = await refreshAccessToken();
  config.headers.Authorization = `Bearer ${token}`;
  return http.request(config);
}

/** 刷新接口自身失败时不再递归刷新 */
function canRefresh(config?: RetryConfig) {
  return !!config && !config._retried && !String(config.url || "").includes("/auth/refresh");
}

http.interceptors.request.use((config) => {
  const token = tokenProvider?.() || "";
  if (token) config.headers.Authorization = `Bearer ${token}`;
  progressStart();
  return config;
});

http.interceptors.response.use(
  (resp) => {
    progressDone();
    const env = resp.data as { code: number; msg: string; data: unknown };
    if (env && typeof env === "object" && "code" in env) {
      if (env.code === 0) return env.data as never;
      /* mock 适配器用 HTTP 200 + code=401 表达未登录，真实后端走下方 http 401 分支 */
      const config = resp.config as RetryConfig;
      if (env.code === 401 && canRefresh(config)) {
        return replayAfterRefresh(config).catch(() => {
          onUnauthorized?.();
          return Promise.reject(new AppError(401, "登录已失效"));
        });
      }
      if (env.code === 401) onUnauthorized?.();
      return Promise.reject(new AppError(env.code, env.msg || "请求失败"));
    }
    return resp.data as never;
  },
  (err: AxiosError<{ msg?: string }>) => {
    progressDone();
    const status = err.response?.status;
    const config = err.config as RetryConfig | undefined;
    if (status === 401 && canRefresh(config)) {
      return replayAfterRefresh(config as RetryConfig).catch(() => {
        onUnauthorized?.();
        return Promise.reject(new AppError(401, "登录已失效"));
      });
    }

    const msg = err.response?.data?.msg || err.message || "网络异常";
    if (status === 401) onUnauthorized?.();
    return Promise.reject(new AppError(status ?? -1, msg));
  },
);

/* 类型化请求助手：拦截器已解包，这里只做类型收窄 */
export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  return (await http.get(url, { params })) as T;
}
export async function post<T>(url: string, body?: unknown): Promise<T> {
  return (await http.post(url, body)) as T;
}
export async function patch<T>(url: string, body?: unknown): Promise<T> {
  return (await http.patch(url, body)) as T;
}
export async function del<T>(url: string): Promise<T> {
  return (await http.delete(url)) as T;
}
