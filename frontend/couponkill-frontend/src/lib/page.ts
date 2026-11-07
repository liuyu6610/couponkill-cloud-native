import type { Page } from '../types/api'

/** 判断是否分页信封 data：`{items: [...]}` */
function hasItems<T>(raw: unknown): raw is { items: T[]; total?: number; page?: number; size?: number } {
  return !!raw && typeof raw === 'object' && Array.isArray((raw as { items?: unknown }).items)
}

/**
 * 把列表接口的 data 规整成数组：兼容裸数组与分页信封 `{items,total,page,size}`，
 * null/undefined 视为空列表。后端对「全量小列表」（可用券、我的预约…）用哪种形状都不影响页面。
 */
export function toList<T>(raw: T[] | { items: T[] } | null | undefined): T[] {
  if (Array.isArray(raw)) return raw
  return hasItems<T>(raw) ? raw.items : []
}

/**
 * 把分页接口的 data 规整成 Page：兼容分页信封与裸数组（裸数组时 total 取「已知下界」：
 * 满页则假设还有下一页，避免分页器显示假总数）。
 */
export function toPage<T>(raw: Page<T> | T[] | null | undefined, page: number, size: number): Page<T> {
  if (hasItems<T>(raw)) {
    const items = raw.items
    return {
      items,
      total: typeof raw.total === 'number' ? raw.total : (page - 1) * size + items.length,
      page: typeof raw.page === 'number' ? raw.page : page,
      size: typeof raw.size === 'number' ? raw.size : size,
    }
  }
  const items = Array.isArray(raw) ? raw : []
  const known = (page - 1) * size + items.length
  return { items, total: items.length < size ? known : known + 1, page, size }
}
