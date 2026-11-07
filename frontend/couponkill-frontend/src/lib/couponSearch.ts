/** 券列表路由；关键字以其 URL 查询参数 `?search=` 为单一真源（CouponList 读写同一个参数） */
export const COUPON_LIST_PATH = '/coupons'
export const COUPON_SEARCH_PARAM = 'search'

/**
 * 顶部搜索框应显示的关键字。
 *
 * - 在券列表页：跟随 URL `?search=`（与 CouponList 一样 trim），列表页里改/清关键字时顶部框同步；
 * - 其它页面（含券详情 /coupons/:id）：返回空串，离开搜索结果后不再残留旧关键字。
 *
 * @param pathname 当前路由 pathname
 * @param search 当前路由 search（含或不含前导 `?` 均可）
 */
export function headerSearchKeyword(pathname: string, search: string): string {
  if (pathname.replace(/\/+$/, '') !== COUPON_LIST_PATH) return ''
  return (new URLSearchParams(search).get(COUPON_SEARCH_PARAM) ?? '').trim()
}
