/* 纯格式化工具：跨端可复用（未来小程序也可用，勿引入 DOM 依赖以外的内容） */

export function fmtDateTime(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function fmtNumber(n: number): string {
  return n.toLocaleString("zh-CN");
}

/** 相对时间：刚 / n 分钟前 / n 小时前 / n 天前，超过 30 天回落为日期 */
export function fromNow(iso: string): string {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(diff)) return "—";
  const min = Math.floor(diff / 60000);
  if (min < 1) return "刚刚";
  if (min < 60) return `${min} 分钟前`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} 小时前`;
  const day = Math.floor(hr / 24);
  if (day < 30) return `${day} 天前`;
  return fmtDateTime(iso).slice(0, 10);
}

/** 配额展示：0 = 不限（readme 契约） */
export function fmtQuota(n: number, unit: string): string {
  return n === 0 ? "不限" : `${n.toLocaleString("zh-CN")} ${unit}`;
}

/** 到期倒计时：已过期 n 天 / 今天到期 / n 天后到期；90 天内按天，更远按月 */
export function untilNow(iso: string): string {
  if (!iso) return "—";
  const target = new Date(iso).getTime();
  if (Number.isNaN(target)) return "—";
  const days = Math.ceil((target - Date.now()) / 86_400_000);
  if (days < 0) return `已过期 ${Math.abs(days)} 天`;
  if (days === 0) return "今天到期";
  if (days <= 90) return `${days} 天后到期`;
  return `还剩 ${Math.round(days / 30)} 个月`;
}

/** 读取 CSS 变量（图表颜色跟随主题用） */
export function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function pct(used: number, total: number): number {
  if (!total) return 0;
  return Math.min(100, Math.round((used / total) * 100));
}
