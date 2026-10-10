/** 离底部小于这一截仍算「在看最新」，继续跟着往下走 */
export const CAT_FOLLOW_SLACK = 48

/** 视口是不是贴在内容底部。内容还没高出视口时也算在底部。 */
export function nearBottom(scrollTop: number, scrollHeight: number, clientHeight: number, slack = CAT_FOLLOW_SLACK): boolean {
  if (!Number.isFinite(scrollTop) || !Number.isFinite(scrollHeight) || !Number.isFinite(clientHeight)) return true
  if (scrollHeight <= clientHeight + 1) return true
  return scrollHeight - clientHeight - scrollTop <= slack
}

/**
 * 程序把滚动条拉到底时不改变「跟不跟随」。
 * 只有用户自己滚动之后，才按当时是不是在底部决定。
 */
export function keepFollowing(userMoved: boolean, atBottom: boolean, wasFollowing: boolean): boolean {
  if (!userMoved) return wasFollowing
  return atBottom
}
