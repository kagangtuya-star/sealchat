import type { Dice3DWorldConfig } from '@/types'

export const createDefaultDice3DWorldConfig = (): Dice3DWorldConfig => ({
  version: 1,
  platformStyleId: '',
  enabled: true,
  surfaceMode: 'auto',
  customSurface: { x: 0.1, y: 0.1, width: 0.8, height: 0.8 },
  defaultSkin: {
    faceBackground: '#f5f6fa', faceForeground: '#111827', edgeColor: '#d1d5db', outlineColor: '#d1d5db',
    roughness: 0.72, metalness: 0.05, scale: 1, textures: {},
  },
  motion: {
    speed: 1, throwForce: 1, wallBounce: 0.48, entryEdge: 'random',
    lingerMs: 8000, maxDice: 60, interactive: true,
  },
  audio: { enabled: true, volume: 0.65 },
  botRules: [
    {
      id: 'seal-annot', name: '海豹注解式', enabled: true,
      // 2[1d6] / 6[1d8]：复合掷骰结果的可靠面值来源
      pattern: String.raw`(?i)(?P<values>\d+)\[(?P<count>\d*)d(?P<sides>\d+)\]`,
      countGroup: 'count', sidesGroup: 'sides', valuesGroup: 'values',
      valueSeparatorPattern: String.raw`\+`, priority: 10,
    },
    {
      id: 'seal-standard', name: '海豹标准', enabled: true,
      // 1d100=42 / [2d6=1+2]；后端会丢弃「点数后紧跟 [」的误匹配
      pattern: String.raw`(?i)(?:\[|\b)(?P<count>\d*)d(?P<sides>\d+)=(?P<values>\d+(?:\+\d+)*)(?:\]|\b)`,
      countGroup: 'count', sidesGroup: 'sides', valuesGroup: 'values',
      valueSeparatorPattern: String.raw`\+`, priority: 0,
    },
  ],
})
