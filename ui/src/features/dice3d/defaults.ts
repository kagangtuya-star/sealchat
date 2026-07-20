import type { Dice3DWorldConfig } from '@/types'

export const createDefaultDice3DWorldConfig = (): Dice3DWorldConfig => ({
  version: 1,
  platformStyleId: '',
  enabled: true,
  surfaceMode: 'auto',
  customSurface: { x: 0.1, y: 0.1, width: 0.8, height: 0.8 },
  defaultSkin: {
    faceBackground: '#f5f6fa', faceForeground: '#111827', edgeColor: '#d1d5db',
    roughness: 0.72, metalness: 0.05, scale: 1, textures: {},
  },
  motion: {
    speed: 1, throwForce: 1, wallBounce: 0.48, entryEdge: 'random',
    lingerMs: 8000, maxDice: 60, interactive: true,
  },
  audio: { enabled: true, volume: 0.65 },
  botRules: [{
    id: 'seal-standard', name: '海豹骰标准', enabled: true,
    pattern: String.raw`(?i)\[(?P<count>\d*)d(?P<sides>\d+)=(?P<values>\d+(?:\+\d+)*)\]`,
    countGroup: 'count', sidesGroup: 'sides', valuesGroup: 'values',
    valueSeparatorPattern: String.raw`\+`, priority: 0,
  }],
})
