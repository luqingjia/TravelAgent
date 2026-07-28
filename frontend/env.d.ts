/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** API 根地址；为空时使用同源路径（开发期依赖 Vite /api 代理）。 */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
