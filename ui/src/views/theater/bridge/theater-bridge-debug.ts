const THEATER_BRIDGE_DEBUG_STORAGE_KEY = 'sealchat.debug.theater-bridge'

type TheaterBridgeDebugCommandValue = boolean | 'on' | 'off' | 'status'

let enabled = false
let installed = false

const readStoredValue = () => {
  try {
    return globalThis.localStorage?.getItem(THEATER_BRIDGE_DEBUG_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

const writeStoredValue = (value: boolean) => {
  try {
    if (value) globalThis.localStorage?.setItem(THEATER_BRIDGE_DEBUG_STORAGE_KEY, '1')
    else globalThis.localStorage?.removeItem(THEATER_BRIDGE_DEBUG_STORAGE_KEY)
  } catch {
    // Runtime logging still works when storage is unavailable.
  }
}

enabled = readStoredValue()

export const isTheaterBridgeDebugEnabled = () => enabled

export const setTheaterBridgeDebugEnabled = (value: boolean) => {
  enabled = value
  writeStoredValue(value)
  return enabled
}

export const installTheaterBridgeDebugConsoleCommand = () => {
  if (installed || typeof window === 'undefined') return
  installed = true
  window.addEventListener('storage', (event) => {
    if (event.key === THEATER_BRIDGE_DEBUG_STORAGE_KEY) enabled = event.newValue === '1'
  })
  window.sealchatTheaterBridgeDebug = (value: TheaterBridgeDebugCommandValue = 'status') => {
    if (value === true || value === 'on') setTheaterBridgeDebugEnabled(true)
    if (value === false || value === 'off') setTheaterBridgeDebugEnabled(false)
    console.info(`[theater-bridge] debug ${enabled ? 'enabled' : 'disabled'}`)
    return enabled
  }
}

declare global {
  interface Window {
    sealchatTheaterBridgeDebug: (value?: TheaterBridgeDebugCommandValue) => boolean
  }
}
