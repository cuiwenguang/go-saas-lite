import { ref } from "vue";

/* 顶栏请求进度条（nprogress 的极简自实现，无依赖）
 * 用法：请求发出 start()，完成 done()；并发请求计数，最后一个完成才收尾 */
const progress = ref(0);
const visible = ref(false);
let pending = 0;
let timer: ReturnType<typeof setInterval> | null = null;

function creep() {
  if (timer) clearInterval(timer);
  timer = setInterval(() => {
    progress.value = Math.min(80, progress.value + (90 - progress.value) * 0.12);
  }, 120);
}

export function useProgress() {
  return { progress, visible };
}

export function progressStart() {
  pending++;
  if (pending === 1) {
    visible.value = true;
    progress.value = 12;
    creep();
  }
}

export function progressDone() {
  pending = Math.max(0, pending - 1);
  if (pending === 0) {
    if (timer) clearInterval(timer);
    timer = null;
    progress.value = 100;
    setTimeout(() => {
      visible.value = false;
      progress.value = 0;
    }, 320);
  }
}
