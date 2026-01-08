<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import { useWebSocket, type LogMessage, type ServiceStatus } from './composables/useWebSocket'

const MAX_LOGS = 5000

const { status, services, sendCommand, onMessage } = useWebSocket()

const logs = ref<LogMessage[]>([])
const activeService = ref('all')
const searchQuery = ref('')
const levelFilter = ref('all')
const autoScroll = ref(true)
const logsContainer = ref<HTMLElement | null>(null)
const toast = ref<{ message: string; type: 'success' | 'error' } | null>(null)

// Service name -> full status info
const serviceStatusMap = computed(() => {
  const map = new Map<string, { running: boolean; status: string; type?: string }>()
  services.value.forEach(s => map.set(s.name, { running: s.running, status: s.status, type: s.type }))
  return map
})

const serviceList = computed(() => {
  const names = new Set(logs.value.map(l => l.service))
  services.value.forEach(s => names.add(s.name))
  return Array.from(names).sort()
})

const filteredLogs = computed(() => {
  return logs.value.filter(log => {
    if (activeService.value !== 'all' && log.service !== activeService.value) {
      return false
    }
    if (levelFilter.value !== 'all' && log.level !== levelFilter.value) {
      return false
    }
    if (searchQuery.value) {
      const text = `${log.service} ${log.message}`.toLowerCase()
      if (!text.includes(searchQuery.value.toLowerCase())) {
        return false
      }
    }
    return true
  })
})

const canControl = computed(() => activeService.value !== 'all')

const isActiveServiceRunning = computed(() => {
  if (activeService.value === 'all') return false
  const info = serviceStatusMap.value.get(activeService.value)
  if (!info) return false

  // Service is "running" if:
  // 1. running flag is true, OR
  // 2. status is 'running' or 'waiting' (interval services waiting for next run)
  return info.running || info.status === 'running' || info.status === 'waiting'
})

function showToast(message: string, type: 'success' | 'error') {
  toast.value = { message, type }
  setTimeout(() => {
    toast.value = null
  }, 3000)
}

onMessage((msg) => {
  if (msg.type === 'log') {
    logs.value.push(msg as LogMessage)
    if (logs.value.length > MAX_LOGS) {
      logs.value = logs.value.slice(-MAX_LOGS)
    }
    if (autoScroll.value) {
      nextTick(() => {
        if (logsContainer.value) {
          logsContainer.value.scrollTop = logsContainer.value.scrollHeight
        }
      })
    }
  } else if (msg.type === 'response') {
    if (msg.success) {
      showToast(msg.message || 'Success', 'success')
      // Refresh status after action
      setTimeout(() => sendCommand({ action: 'status' }), 500)
    } else if (msg.error) {
      showToast(msg.error, 'error')
    }
  }
})

function handleScroll() {
  if (logsContainer.value) {
    const { scrollTop, scrollHeight, clientHeight } = logsContainer.value
    autoScroll.value = scrollTop + clientHeight >= scrollHeight - 50
  }
}

function clearLogs() {
  logs.value = []
  sendCommand({ action: 'clear', service: activeService.value === 'all' ? '' : activeService.value })
}

function stopService() {
  if (activeService.value !== 'all') {
    sendCommand({ action: 'stop', service: activeService.value })
  }
}

function startService() {
  if (activeService.value !== 'all') {
    sendCommand({ action: 'start', service: activeService.value })
  }
}

function restartService() {
  if (activeService.value !== 'all') {
    sendCommand({ action: 'restart', service: activeService.value })
  }
}

function formatTime(timestamp: string) {
  const date = new Date(timestamp)
  return date.toLocaleTimeString('en-US', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

function getServiceColor(service: string) {
  const colors = ['blue', 'green', 'yellow', 'magenta', 'cyan', 'red']
  let hash = 0
  for (let i = 0; i < service.length; i++) {
    hash = ((hash << 5) - hash) + service.charCodeAt(i)
  }
  return colors[Math.abs(hash) % colors.length]
}

function getServiceStatusColor(service: string): 'green' | 'red' | 'yellow' | 'gray' {
  const info = serviceStatusMap.value.get(service)
  if (!info) return 'gray'

  // Map status to color
  switch (info.status) {
    case 'running':
      return 'green'
    case 'completed':
      return 'green' // Successfully completed (oneshot)
    case 'waiting':
      return 'yellow' // Interval waiting for next run
    case 'failed':
      return 'red'
    case 'stopped':
      return 'gray'
    default:
      return info.running ? 'green' : 'gray'
  }
}
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Toast -->
    <Transition name="toast">
      <div
        v-if="toast"
        class="absolute left-1/2 top-4 z-50 -translate-x-1/2 rounded-lg px-4 py-2 text-sm font-medium shadow-lg"
        :class="toast.type === 'success' ? 'bg-[#2d7d46] text-white' : 'bg-[#a31515] text-white'"
      >
        {{ toast.message }}
      </div>
    </Transition>

    <!-- Toolbar -->
    <div class="flex flex-col border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]">
      <!-- Row 1: Tabs + Status -->
      <div class="flex items-center justify-between px-3 py-1.5 border-b border-[var(--color-border)]/50">
        <!-- Tabs -->
        <div class="flex gap-1 overflow-x-auto flex-1 min-w-0">
          <button
            class="rounded px-2 py-1 text-xs whitespace-nowrap transition-colors shrink-0"
            :class="activeService === 'all'
              ? 'bg-[var(--color-accent)] text-white'
              : 'text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]'"
            @click="activeService = 'all'"
          >
            All
          </button>
          <button
            v-for="service in serviceList"
            :key="service"
            class="flex items-center gap-1 rounded px-2 py-1 text-xs whitespace-nowrap transition-colors shrink-0"
            :class="activeService === service
              ? 'bg-[var(--color-accent)] text-white'
              : 'text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]'"
            @click="activeService = service"
          >
            <!-- Status dot -->
            <span
              class="h-1.5 w-1.5 rounded-full"
              :class="{
                'bg-[#4ec9b0]': getServiceStatusColor(service) === 'green',
                'bg-[#f48771]': getServiceStatusColor(service) === 'red',
                'bg-[#dcdcaa]': getServiceStatusColor(service) === 'yellow',
                'bg-[#858585]': getServiceStatusColor(service) === 'gray',
              }"
            />
            {{ service }}
          </button>
        </div>

        <!-- Status badge -->
        <span
          class="rounded px-2 py-0.5 text-[10px] uppercase shrink-0 ml-2"
          :class="{
            'bg-[#2d4a3e] text-[#4ec9b0]': status === 'connected',
            'bg-[#4a2d2d] text-[#f48771]': status === 'disconnected',
            'bg-[#4a4a2d] text-[#dcdcaa]': status === 'connecting',
          }"
        >
          {{ status }}
        </span>
      </div>

      <!-- Row 2: Controls -->
      <div class="flex items-center gap-2 px-3 py-1.5 overflow-x-auto">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Filter..."
          class="rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-2 py-1 text-xs text-[var(--color-text-primary)] w-24 min-w-[80px] focus:border-[var(--color-accent)] focus:outline-none"
        >

        <select
          v-model="levelFilter"
          class="rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-1 py-1 text-xs text-[var(--color-text-primary)]"
        >
          <option value="all">All</option>
          <option value="error">Error</option>
          <option value="warn">Warn</option>
          <option value="info">Info</option>
          <option value="debug">Debug</option>
        </select>

        <div class="flex items-center gap-1 ml-auto">
          <button
            :disabled="!canControl || !isActiveServiceRunning"
            class="rounded bg-[var(--color-danger)] px-2 py-1 text-xs text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
            title="Stop service"
            @click="stopService"
          >
            Stop
          </button>

          <button
            :disabled="!canControl || isActiveServiceRunning"
            class="rounded bg-[var(--color-success)] px-2 py-1 text-xs text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
            title="Start service"
            @click="startService"
          >
            Start
          </button>

          <button
            :disabled="!canControl"
            class="rounded bg-[var(--color-accent)] px-2 py-1 text-xs text-white transition-colors hover:bg-[var(--color-accent-hover)] disabled:cursor-not-allowed disabled:opacity-40"
            title="Restart service"
            @click="restartService"
          >
            Restart
          </button>

          <button
            class="rounded bg-[var(--color-bg-tertiary)] px-2 py-1 text-xs text-[var(--color-text-primary)] transition-colors hover:opacity-90"
            @click="clearLogs"
          >
            Clear
          </button>
        </div>
      </div>
    </div>

    <!-- Logs -->
    <div
      ref="logsContainer"
      class="flex-1 overflow-y-auto py-2"
      @scroll="handleScroll"
    >
      <div
        v-if="filteredLogs.length === 0"
        class="flex h-full flex-col items-center justify-center text-[var(--color-text-secondary)]"
      >
        <p>Waiting for devir logs...</p>
        <p class="mt-2 text-xs text-[var(--color-text-secondary)]/60">
          Make sure devir is running with WebSocket enabled (port 9222)
        </p>
      </div>

      <div
        v-for="(log, index) in filteredLogs"
        :key="index"
        class="flex px-3 py-0.5 leading-relaxed hover:bg-[var(--color-bg-secondary)]"
      >
        <span class="mr-2 shrink-0 text-[#6a9955]">{{ formatTime(log.time) }}</span>
        <span
          class="mr-2 shrink-0 rounded px-1 font-medium"
          :class="`bg-[var(--color-service-${getServiceColor(log.service)})]`"
        >
          {{ log.service }}
        </span>
        <span
          class="flex-1 whitespace-pre-wrap break-all"
          :class="{
            'text-[#f48771]': log.level === 'error',
            'text-[#dcdcaa]': log.level === 'warn',
            'text-[var(--color-text-secondary)]': log.level === 'debug',
          }"
        >
          {{ log.message }}
        </span>
      </div>
    </div>
  </div>
</template>

<style>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.3s ease;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translate(-50%, -20px);
}
</style>
