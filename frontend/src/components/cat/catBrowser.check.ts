import { browserDocument, browserOpen, browserURL, readBrowserNav, BROWSER_NAV } from './catBrowser'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catBrowserChecks(eq: Eq): void {
  eq('网址补上 http', browserURL('localhost:5173/a'), 'http://localhost:5173/a')
  eq('保留 https', browserURL('https://example.com/x'), 'https://example.com/x')
  eq('不要别的协议', browserURL('javascript:alert(1)'), '')
  eq('空地址', browserURL('   '), '')
  const q = (s: string) => 'https://www.google.com/search?q=' + encodeURIComponent(s)
  eq('中文去谷歌', browserOpen('猫'), q('猫'))
  eq('句子去谷歌', browserOpen('你好 世界'), q('你好 世界'))
  eq('域名仍打开', browserOpen('example.com/a'), 'http://example.com/a')
  eq('本机地址仍打开', browserOpen('localhost:5173/a'), 'http://localhost:5173/a')
  eq('写了协议的地址仍打开', browserOpen('https://example.com/x'), 'https://example.com/x')
  eq('脚本地址丢掉', browserOpen('javascript:alert(1)'), '')
  const opened = browserOpen('例.中国')
  eq('带点的中文域名不是搜索', [opened.startsWith('http://'), opened.includes('google.com/search')], [true, false])
  const doc = browserDocument('<html><head><title>a</title></head><body>hi</body></html>', 'https://ex.com/a', 'tok123')
  eq('页面带上基准地址和点选', [doc.includes('href="https://ex.com/a"'), doc.includes('tok123'), doc.includes('data-cat-pick')], [true, true, true])
  eq('认页面里点开的链接', readBrowserNav({ source: BROWSER_NAV, token: 'tok123', kind: 'open', href: 'https://ex.com/b' }, 'tok123'), 'https://ex.com/b')
  eq('不认别的页面', readBrowserNav({ source: BROWSER_NAV, token: 'other', kind: 'open', href: 'https://ex.com/b' }, 'tok123'), '')
}
