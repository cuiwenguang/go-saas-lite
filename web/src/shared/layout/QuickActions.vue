<script setup lang="ts">
import { Bell, Brush, Check, MagicStick, Moon, Sunny } from "@element-plus/icons-vue";
import { useAppStore } from "@/shared/stores/app";

const props = defineProps<{ side: "admin" | "tenant" }>();

const app = useAppStore();

/* 消息中心：原型数据，仅演示已读交互；平台/租户按端区分内容 */
const allRead = ref(false);
const notices = computed(() => {
  const admin = [
    { title: "租户「星野科技」已开通", detail: "初始密码已生成，请及时交付给管理员", time: "10 分钟前", tone: "success" },
    { title: "Acme 存储用量达到 80%", detail: "建议关注用量趋势或调整套餐配额", time: "1 小时前", tone: "warning" },
    { title: "本周平台周报已生成", detail: "本周新增 3 个租户，MCP 调用量增长 12%", time: "昨天", tone: "brand" },
  ];
  const tenant = [
    { title: "存储用量已达配额 80%", detail: "可清理历史数据或联系平台调整套餐", time: "30 分钟前", tone: "warning" },
    { title: "「李四」已加入团队", detail: "初始密码已通过一次性方式生成，请提醒首次登录修改", time: "2 小时前", tone: "success" },
    { title: "API Key「集成联调」将于 7 天后过期", detail: "如仍在使用，请提前轮换，避免影响业务调用", time: "昨天", tone: "brand" },
  ];
  return props.side === "admin" ? admin : tenant;
});

function markAllRead() {
  allRead.value = true;
  ElMessage.success("全部消息已标为已读");
}

/* AI 助手：本地 mock 对话，不接真实模型 */
const aiOpen = ref(false);
const aiInput = ref("");
const chats = ref<{ role: "assistant" | "user"; text: string }[]>([
  { role: "assistant", text: "你好，我是工作区 AI 助手。可以帮你汇总平台数据、解释配额用量，或生成操作指引。" },
]);
const suggestions = ["汇总本周租户情况", "哪些租户需要关注？", "如何开通新租户？"];

function ask(text: string) {
  const question = text.trim();
  if (!question) return;
  chats.value.push({ role: "user", text: question });
  aiInput.value = "";
  const answer = question.includes("开通")
    ? "点击左侧「租户管理」，再点击右上角「开通租户」。填写名称、子域名和管理员邮箱即可，初始密码只会展示一次。"
    : question.includes("关注")
      ? "建议优先关注存储用量达到 80% 的 Acme，以及当前处于停用状态的租户。你可以在租户管理中按状态快速筛选。"
      : "本周新增 3 个租户，当前共有 8 个租户，其中 6 个活跃。MCP 调用量较上周增长 12%，整体运行正常。";
  chats.value.push({ role: "assistant", text: answer });
}
</script>

<template>
  <div class="quick-actions">
    <el-dropdown trigger="click" placement="bottom-end">
      <el-button text class="action-btn" aria-label="外观设置">
        <el-icon :size="18"><Brush /></el-icon>
      </el-button>
      <template #dropdown>
        <el-dropdown-menu class="skin-menu">
          <el-dropdown-item @click="app.setDark(false)">
            <el-icon><Sunny /></el-icon><span>亮色模式</span>
            <el-icon v-if="!app.dark" class="choice-check"><Check /></el-icon>
          </el-dropdown-item>
          <el-dropdown-item @click="app.setDark(true)">
            <el-icon><Moon /></el-icon><span>暗色模式</span>
            <el-icon v-if="app.dark" class="choice-check"><Check /></el-icon>
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>

    <el-popover placement="bottom-end" :width="360" trigger="click" popper-class="notice-popover">
      <template #reference>
        <el-badge :is-dot="!allRead" class="notice-badge">
          <el-button text class="action-btn" aria-label="消息中心">
            <el-icon :size="18"><Bell /></el-icon>
          </el-button>
        </el-badge>
      </template>
      <div class="notice-head">
        <strong>消息通知</strong>
        <el-button text size="small" :disabled="allRead" @click="markAllRead">全部已读</el-button>
      </div>
      <div class="notice-list" :class="{ read: allRead }">
        <div v-for="notice in notices" :key="notice.title" class="notice-item">
          <span class="notice-dot" :class="notice.tone"></span>
          <div class="notice-content">
            <p class="notice-title">{{ notice.title }}</p>
            <p class="notice-detail">{{ notice.detail }}</p>
            <span class="notice-time">{{ notice.time }}</span>
          </div>
        </div>
      </div>
    </el-popover>

    <el-tooltip v-if="side === 'admin'" content="AI 助手" placement="bottom">
      <el-button text class="action-btn ai-btn" aria-label="AI 助手" @click="aiOpen = true">
        <el-icon :size="19"><MagicStick /></el-icon>
      </el-button>
    </el-tooltip>

    <el-drawer v-if="side === 'admin'" v-model="aiOpen" title="AI 助手" size="400px" append-to-body class="ai-drawer">
      <div class="assistant">
        <div class="chat-list">
          <div v-for="(chat, index) in chats" :key="index" class="chat-row" :class="chat.role">
            <div v-if="chat.role === 'assistant'" class="assistant-mark"><el-icon><MagicStick /></el-icon></div>
            <div class="chat-bubble">{{ chat.text }}</div>
          </div>
        </div>
        <div class="suggestions">
          <button v-for="item in suggestions" :key="item" type="button" @click="ask(item)">{{ item }}</button>
        </div>
        <div class="ask-box">
          <el-input v-model="aiInput" placeholder="输入你的问题…" clearable @keyup.enter="ask(aiInput)" />
          <el-button type="primary" :disabled="!aiInput.trim()" @click="ask(aiInput)">发送</el-button>
        </div>
        <p class="ai-tip">界面原型 · 当前为模拟回答</p>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.quick-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
.action-btn {
  width: 34px;
  height: 34px;
  margin: 0;
  padding: 0;
  color: var(--el-text-color-regular);
  border-radius: 9px;
}
.action-btn:hover {
  color: var(--brand);
  background: var(--el-fill-color-light);
}
.ai-btn {
  color: var(--brand);
  background: var(--el-color-primary-light-9);
}
.notice-badge {
  display: flex;
  align-items: center;
}
.notice-badge :deep(.el-badge__content.is-dot) {
  top: 7px;
  right: 8px;
  width: 6px;
  height: 6px;
  border: 1px solid var(--page-bg);
}
.choice-check {
  margin-left: auto;
  color: var(--brand);
}
.notice-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.notice-list {
  transition: opacity 0.2s ease;
}
.notice-list.read {
  opacity: 0.55;
}
.notice-item {
  display: flex;
  gap: 10px;
  padding: 13px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.notice-item:last-child {
  border-bottom: 0;
  padding-bottom: 2px;
}
.notice-dot {
  width: 7px;
  height: 7px;
  margin-top: 6px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--brand);
}
.notice-dot.success { background: var(--el-color-success); }
.notice-dot.warning { background: var(--el-color-warning); }
.notice-content { min-width: 0; }
.notice-title {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.notice-detail {
  margin: 4px 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-regular);
}
.notice-time {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.assistant {
  height: 100%;
  display: flex;
  flex-direction: column;
}
.chat-list {
  flex: 1;
  overflow-y: auto;
  padding: 2px 0 16px;
}
.chat-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 14px;
}
.chat-row.user {
  justify-content: flex-end;
}
.assistant-mark {
  width: 28px;
  height: 28px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--brand);
  background: var(--el-color-primary-light-9);
}
.chat-bubble {
  max-width: 82%;
  padding: 10px 12px;
  border-radius: 4px 12px 12px 12px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-size: 13px;
  line-height: 1.7;
}
.chat-row.user .chat-bubble {
  color: #fff;
  background: var(--brand-strong);
  border-radius: 12px 4px 12px 12px;
}
.suggestions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}
.suggestions button {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 999px;
  padding: 6px 10px;
  background: var(--el-bg-color);
  color: var(--el-text-color-regular);
  font-size: 12px;
  cursor: pointer;
}
.suggestions button:hover {
  color: var(--brand);
  border-color: var(--brand);
}
.ask-box {
  display: flex;
  gap: 8px;
}
.ai-tip {
  margin: 10px 0 0;
  text-align: center;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}
</style>
