/** 每个实例只拥有自己的数据；上下文（scope/channel/timezone）与月份共同隔离缓存。 */
export function createCalendarActiveDayCache(load: (month: string, context: string) => Promise<string[]>) {
  const days = new Map<string, Set<string>>()
  const pending = new Map<string, Promise<void>>()
  return {
    get(context: string): Record<string, Set<string>> {
      const result: Record<string, Set<string>> = {}
      for (const [key, value] of days) {
        if (key.startsWith(`${context}|`)) result[key.slice(context.length + 1)] = value
      }
      return result
    },
    async ensure(month: string, context: string) {
      const key = `${context}|${month}`
      if (days.has(key)) return
      let request = pending.get(key)
      if (!request) {
        request = load(month, context).then(value => { days.set(key, new Set(value)) })
          .finally(() => { pending.delete(key) })
        pending.set(key, request)
      }
      await request
    },
  }
}
