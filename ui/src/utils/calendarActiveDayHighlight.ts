export interface ApplyCalendarActiveDayHighlightsOptions {
  panel: HTMLElement | null
  activeDaySetByMonth: Record<string, Set<string>>
  locale?: string
}

const ACTIVE_DAY_CLASS = 'sc-calendar-active-day'
const ACTIVE_DAY_ATTR = 'data-sc-calendar-active-day'

const buildDayKey = (date: Date) => {
  const year = String(date.getFullYear()).padStart(4, '0')
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

// Naive 2.40.1 的 data-n-date 仅是布尔标记，没有日期值。
// 标题是 locale 格式化的月/年；兼容项目的中、英、日文，不假定标题顺序。
export const parsePanelMonthKey = (text: string, locale = 'en-US') => {
  const year = text.match(/\b\d{4}\b/)?.[0]
  if (!year) return ''
  const rest = text.replace(year, '').replace(/年/g, '').trim().toLocaleLowerCase(locale)
  const numeric = rest.match(/(?:^|\D)(\d{1,2})\s*月/)
  let month = numeric ? Number(numeric[1]) : 0
  if (!month) {
    for (let index = 0; index < 12; index++) {
      for (const width of ['long', 'short'] as const) {
        const name = new Intl.DateTimeFormat(locale, { month: width, timeZone: 'UTC' })
          .format(new Date(Date.UTC(2000, index, 1))).toLocaleLowerCase(locale)
        if (rest.replace(/[.,]/g, '').trim() === name.replace(/[.,]/g, '').trim()) month = index + 1
      }
    }
  }
  return month >= 1 && month <= 12 ? `${year}-${String(month).padStart(2, '0')}` : ''
}

function calendarCells(calendar: HTMLElement) {
  return Array.from(calendar.querySelectorAll<HTMLElement>('[data-n-date].n-date-panel-date'))
}

function calendarMonth(calendar: HTMLElement, locale: string) {
  const title = calendar.querySelector('.n-date-panel-month__month-year, .n-date-panel-month__text')?.textContent || ''
  return parsePanelMonthKey(title, locale)
}

function calendarDates(calendar: HTMLElement, month: string) {
  const cells = calendarCells(calendar)
  // 从本月的 1 号定位第一格；无需假定 locale 的每周起点或补位格数量。
  const first = cells.findIndex(cell => !cell.classList.contains('n-date-panel-date--excluded') && cell.textContent?.trim() === '1')
  if (first < 0) return []
  const base = new Date(`${month}-01T12:00:00`)
  return cells.map((cell, index) => {
    const date = new Date(base)
    date.setDate(1 + index - first)
    return { cell, day: buildDayKey(date) }
  })
}

export function collectCalendarMonthKeys(panel: HTMLElement, locale = 'en-US') {
  const months = new Set<string>()
  panel.querySelectorAll<HTMLElement>('.n-date-panel-calendar').forEach(calendar => {
    const month = calendarMonth(calendar, locale)
    if (!month) return
    months.add(month)
    calendarDates(calendar, month).forEach(({ day }) => months.add(day.slice(0, 7)))
  })
  return [...months]
}

const clearCalendarActiveDayHighlights = (panel: HTMLElement) => {
  panel.querySelectorAll<HTMLElement>(`.${ACTIVE_DAY_CLASS}, [${ACTIVE_DAY_ATTR}]`).forEach((cell) => {
    cell.classList.remove(ACTIVE_DAY_CLASS)
    cell.removeAttribute(ACTIVE_DAY_ATTR)
  })
}

export const applyCalendarActiveDayHighlights = ({
  panel,
  activeDaySetByMonth,
  locale = 'en-US',
}: ApplyCalendarActiveDayHighlightsOptions) => {
  if (!panel) {
    return false
  }
  clearCalendarActiveDayHighlights(panel)
  let highlighted = false
  const calendars = Array.from(panel.querySelectorAll<HTMLElement>('.n-date-panel-calendar'))
  calendars.forEach((calendar) => {
    const monthKey = calendarMonth(calendar, locale)
    if (!monthKey) {
      return
    }
    calendarDates(calendar, monthKey).forEach(({ cell, day }) => {
      const dateMonthKey = day.slice(0, 7)
      const activeDays = activeDaySetByMonth[dateMonthKey]
      if (!activeDays || activeDays.size === 0) {
        return
      }
      if (!activeDays.has(day)) {
        return
      }
      cell.classList.add(ACTIVE_DAY_CLASS)
      cell.setAttribute(ACTIVE_DAY_ATTR, 'true')
      highlighted = true
    })
  })
  return highlighted
}
