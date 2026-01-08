import { ref, onUnmounted } from 'vue'

export interface LogMessage {
  type: 'log'
  time: string
  service: string
  level: string
  message: string
}

export interface StatusMessage {
  type: 'status'
  services: ServiceStatus[]
}

export interface ServiceStatus {
  name: string
  running: boolean
  status: 'running' | 'stopped' | 'completed' | 'failed' | 'waiting'
  type?: 'service' | 'oneshot' | 'interval' | 'http'
  port?: number
  color: string
  icon?: string
}

export interface ResponseMessage {
  type: 'response'
  success: boolean
  message?: string
  error?: string
}

export type WSMessage = LogMessage | StatusMessage | ResponseMessage

const WS_URL = 'ws://localhost:9222/logs'
const RECONNECT_DELAY = 3000

export function useWebSocket() {
  const ws = ref<WebSocket | null>(null)
  const status = ref<'connected' | 'disconnected' | 'connecting'>('disconnected')
  const services = ref<ServiceStatus[]>([])

  let reconnectTimeout: ReturnType<typeof setTimeout> | null = null
  const messageHandlers: ((msg: WSMessage) => void)[] = []

  function connect() {
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout)
      reconnectTimeout = null
    }

    status.value = 'connecting'

    try {
      ws.value = new WebSocket(WS_URL)

      ws.value.onopen = () => {
        status.value = 'connected'
        sendCommand({ action: 'status' })
      }

      ws.value.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as WSMessage

          if (data.type === 'status') {
            services.value = (data as StatusMessage).services
          }

          messageHandlers.forEach(handler => handler(data))
        } catch (e) {
          console.error('Failed to parse message:', e)
        }
      }

      ws.value.onclose = () => {
        status.value = 'disconnected'
        scheduleReconnect()
      }

      ws.value.onerror = () => {
        status.value = 'disconnected'
      }
    } catch (e) {
      status.value = 'disconnected'
      scheduleReconnect()
    }
  }

  function scheduleReconnect() {
    if (!reconnectTimeout) {
      reconnectTimeout = setTimeout(connect, RECONNECT_DELAY)
    }
  }

  function sendCommand(cmd: { action: string; service?: string }) {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) {
      ws.value.send(JSON.stringify(cmd))
    }
  }

  function onMessage(handler: (msg: WSMessage) => void) {
    messageHandlers.push(handler)
  }

  function disconnect() {
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout)
    }
    if (ws.value) {
      ws.value.close()
    }
  }

  onUnmounted(disconnect)

  // Auto-connect
  connect()

  return {
    status,
    services,
    sendCommand,
    onMessage,
    connect,
    disconnect,
  }
}
