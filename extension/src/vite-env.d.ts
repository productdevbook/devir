/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

declare const chrome: {
  devtools: {
    panels: {
      create: (title: string, iconPath: string, pagePath: string, callback?: (panel: unknown) => void) => void
    }
  }
}
