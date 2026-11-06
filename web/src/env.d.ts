/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** "false" 时禁用 mock，走真实后端（需后端已起） */
  readonly VITE_USE_MOCK?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module "*.vue" {
  import type { DefineComponent } from "vue"
  const component: DefineComponent<{}, {}, any>
  export default component
}
