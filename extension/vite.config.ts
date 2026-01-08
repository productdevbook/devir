import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'path'
import { existsSync, readFileSync, writeFileSync, rmSync } from 'fs'

export default defineConfig({
  base: './',
  plugins: [
    vue(),
    tailwindcss(),
    {
      name: 'flatten-output',
      closeBundle() {
        const dist = resolve(__dirname, 'dist')

        // Move src/devtools.html to dist/devtools.html and fix paths
        const devtoolsSrc = resolve(dist, 'src/devtools.html')
        const devtoolsDest = resolve(dist, 'devtools.html')
        if (existsSync(devtoolsSrc)) {
          let content = readFileSync(devtoolsSrc, 'utf-8')
          content = content.replace(/\.\.\//g, './')
          writeFileSync(devtoolsDest, content)
        }

        // Move src/panel/index.html to dist/panel.html and fix paths
        const panelSrc = resolve(dist, 'src/panel/index.html')
        const panelDest = resolve(dist, 'panel.html')
        if (existsSync(panelSrc)) {
          let content = readFileSync(panelSrc, 'utf-8')
          content = content.replace(/\.\.\/\.\.\//g, './')
          writeFileSync(panelDest, content)
        }

        // Clean up src folder
        const srcDir = resolve(dist, 'src')
        if (existsSync(srcDir)) {
          rmSync(srcDir, { recursive: true })
        }
      },
    },
  ],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        panel: resolve(__dirname, 'src/panel/index.html'),
        devtools: resolve(__dirname, 'src/devtools.html'),
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name].[ext]',
      },
    },
  },
})
