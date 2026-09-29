// 不引入测试框架的自检：node scripts/check-json-text.mjs（esbuild 打包后在 node 里跑）。
import { describeServiceError, describeSyntax, errorTokenSpan, startsWithCJK, unescapeText } from './jsonText'

export function runJsonTextChecks(): string[] {
  const fails: string[] = []
  const eq = (name: string, got: unknown, want: unknown) => {
    if (got !== want) fails.push(`${name}: 期望 ${JSON.stringify(want)}，实际 ${JSON.stringify(got)}`)
  }

  // startsWithCJK
  eq('中文开头', startsWithCJK('内容为空'), true)
  eq('前导空白后中文', startsWithCJK('  内容不完整'), true)
  eq('英文开头', startsWithCJK('invalid character'), false)
  eq('英文里夹中文', startsWithCJK("invalid character '中' looking for beginning of value"), false)
  eq('空串', startsWithCJK(''), false)

  // describeSyntax：中文开头原样放行，其余走映射或兜底
  eq('服务端中文原样', describeSyntax('JSON 语法错误：内容为空'), '内容为空')
  eq('中文开头原样', describeSyntax('结束后还有多余内容'), '结束后还有多余内容')
  eq('英文夹中文-值', describeSyntax("invalid character '中' looking for beginning of value"), '这里应该是一个值')
  eq('英文夹中文-逗号', describeSyntax("invalid character '中' after object key:value pair"), '缺少逗号或括号')
  eq('英文夹中文-数组', describeSyntax("invalid character '值' after array element"), '缺少逗号或括号')
  eq('英文夹中文-未知', describeSyntax("something odd with '中'"), '语法错误')
  eq('V8 逗号', describeSyntax("Expected ',' or '}' after property value in JSON at position 5"), '缺少逗号或括号')
  eq('未知英文兜底', describeSyntax('boom'), '语法错误')

  // describeServiceError
  eq('去转义 INVALID_ARGUMENT', describeServiceError('INVALID_ARGUMENT', '不是有效的转义字符串', 'unescape'), '不是有效的转义字符串')
  eq('去转义英文消息也映射', describeServiceError('INVALID_ARGUMENT', 'bad escape', 'unescape'), '不是有效的转义字符串')
  eq('中文 message 直接用', describeServiceError('IO_ERROR', '内容太大，无法处理', 'format'), '内容太大，无法处理')
  eq('英文 message 兜底', describeServiceError('INTERNAL', 'runtime error: nil pointer', 'format'), '处理失败，请检查输入内容')
  eq('夹中文的英文兜底', describeServiceError('INTERNAL', "unexpected '中'", 'compact'), '处理失败，请检查输入内容')
  eq('空 message 兜底', describeServiceError('INTERNAL', '', 'format'), '处理失败，请检查输入内容')
  eq('undefined message 兜底', describeServiceError('INTERNAL', undefined), '处理失败，请检查输入内容')
  for (const [code, msg] of [['INVALID_ARGUMENT', 'x'], ['INTERNAL', 'x'], ['PROCESS_FAILED', '']] as const) {
    const out = describeServiceError(code, msg, 'unescape')
    if (/[A-Z_]{6,}/.test(out) || out.includes('出错了')) fails.push(`文案里不应出现错误码 / 出错了：${out}`)
  }

  // unescapeText 失败时的文案
  let thrown = ''
  try {
    unescapeText('abc\\qdef')
  } catch (e) {
    thrown = (e as Error).message
  }
  eq('去转义非法转义', thrown, '不是有效的转义字符串')
  eq('去转义带引号', unescapeText('"{\\"a\\":1}"'), '{"a":1}')

  // errorTokenSpan
  eq('span 字符串', JSON.stringify(errorTokenSpan('  "twoPass": true,', 2)), JSON.stringify({ start: 2, length: 9 }))
  eq('span 数字到分隔符', JSON.stringify(errorTokenSpan('  "id": 42 "x": 1', 8)), JSON.stringify({ start: 8, length: 2 }))
  eq('span 缺逗号处的下一个键', JSON.stringify(errorTokenSpan('  "id": 42 "x": 1', 11)), JSON.stringify({ start: 11, length: 3 }))
  eq('span 标点', JSON.stringify(errorTokenSpan('a,b', 1)), JSON.stringify({ start: 1, length: 1 }))
  eq('span 越界', JSON.stringify(errorTokenSpan('ab', 99)), JSON.stringify({ start: 1, length: 1 }))
  return fails
}
