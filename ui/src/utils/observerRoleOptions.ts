import type { ChannelIcOocRoleConfig } from '../types'

export interface ObserverRoleOption {
  label: string
  value: string
}

const resolveObserverRoleOrder = (
  value: string,
  config?: Partial<ChannelIcOocRoleConfig> | null,
  rolelessValue = '__roleless__',
) => {
  if (value === rolelessValue) {
    return 3
  }
  if (value && value === config?.icRoleId) {
    return 0
  }
  if (value && value === config?.oocRoleId) {
    return 1
  }
  return 2
}

export const sortObserverRoleOptions = (
  options: ObserverRoleOption[],
  config?: Partial<ChannelIcOocRoleConfig> | null,
  rolelessValue = '__roleless__',
) => {
  return options
    .map((item, index) => ({ item, index }))
    .sort((left, right) => {
      const leftOrder = resolveObserverRoleOrder(left.item.value, config, rolelessValue)
      const rightOrder = resolveObserverRoleOrder(right.item.value, config, rolelessValue)
      if (leftOrder !== rightOrder) {
        return leftOrder - rightOrder
      }
      return left.index - right.index
    })
    .map(({ item }) => item)
}
