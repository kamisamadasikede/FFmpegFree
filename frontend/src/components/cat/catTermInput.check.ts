import { pipeTermKeys } from './catTermInput'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catTermInputChecks(eq: Eq): void {
  const typed = pipeTermKeys('', 'dir')
  eq('管道终端先把字画出来', [typed.line, typed.screen, typed.send], ['dir', 'dir', ''])
  const undone = pipeTermKeys(typed.line, '\b')
  eq('退格抹掉一个字', [undone.line, undone.screen], ['di', '\b \b'])
  const sent = pipeTermKeys('echo 猫', '\r')
  eq('回车送出整行', [sent.line, sent.screen, sent.send], ['', '\r\n', 'echo 猫\r\n'])
  eq('方向键不写进命令', pipeTermKeys('a', '\u001b[A'), { line: 'a', screen: '', send: '' })
}
