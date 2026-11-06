import { defineStore } from "pinia";
import { ref } from "vue";

/* 布局状态：折叠 / 暗色。UI 偏好允许落 localStorage（非业务数据） */

export const useAppStore = defineStore("app", () => {
  const collapsed = ref(false);
  const dark = ref(localStorage.getItem("ui.dark") === "1");

  function setDark(value: boolean) {
    dark.value = value;
    localStorage.setItem("ui.dark", dark.value ? "1" : "0");
    document.documentElement.classList.toggle("dark", dark.value);
  }

  function toggleDark() {
    setDark(!dark.value);
  }

  function toggleCollapsed() {
    collapsed.value = !collapsed.value;
  }

  function init() {
    document.documentElement.classList.toggle("dark", dark.value);
  }

  return { collapsed, dark, setDark, toggleDark, toggleCollapsed, init };
});
