
import { t } from '@/i18n'/** 管道终端不会自己回显。按键画在画面上，敲回车才把整行送出去。 */
export function pipeTermKeys(line: string, data: string): { line: string; screen: string; send: string } {
  if (!data || data.startsWith('\u001b')) return { line, screen: '', send: '' }
  let screen = ''
  let send = ''
  for (const ch of Array.from(data.replace(/\u007f/g, '\b'))) {
    if (ch === '\r' || ch === '\n') {
      screen += '\r\n'
      send += line + '\r\n'
      line = ''
      continue
    }
    if (ch === '\b') {
      const cur = Array.from(line)
      if (cur.length === 0) continue
      cur.pop()
      line = cur.join('')
      screen += '\b \b'
      continue
    }
    if (ch === '\u0003') {
      line = ''
      screen += '^C\r\n'
      send += '\u0003'
      continue
    }
    if (ch < ' ') continue
    line += ch
    screen += ch
  }
  return { line, screen, send }
}
