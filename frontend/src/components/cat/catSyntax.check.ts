import { SYNTAX_LANGUAGES, encodeSyntaxSpans, syntaxLabels, syntaxSpans } from './catSyntax'

type Eq = (name: string, got: unknown, want: unknown) => void

const ofKind = (language: string, text: string, kind: string) =>
  syntaxLabels(language, text).filter((item) => item.endsWith(':' + kind))

const picked = (language: string, text: string, kinds: string[]) =>
  syntaxLabels(language, text).filter((item) => kinds.some((kind) => item.endsWith(':' + kind)))

export function catSyntaxChecks(eq: Eq): void {
  eq(
    '十种语言：C 和 C++ 共用 cpp',
    [...SYNTAX_LANGUAGES],
    ['python', 'cpp', 'java', 'csharp', 'javascript', 'typescript', 'go', 'php', 'rust'],
  )
  eq('纯文本不再标一层', syntaxSpans('plaintext', 'func main() {}\n'), [])
  eq('SQL 不在这层重复标', syntaxSpans('sql', 'select 1\n'), [])

  const go = [
    'package collector',
    '',
    'func TestFetch(t *testing.T) {',
    '\tsrv := httptest.NewServer(nil)',
    '\tvar msg string',
    '\ttype Box struct{}',
    '}',
    '',
    'func (c *Collector) Run() {}',
    '',
  ].join('\n')
  eq('Go 函数名和类型名', picked('go', go, ['function', 'type']), [
    'TestFetch:function',
    'T:type',
    'NewServer:function',
    'string:type',
    'Box:type',
    'Collector:type',
    'Run:function',
  ])
  eq('Go 关键字仍是关键字', ofKind('go', go, 'keyword').filter((item) => item === 'func:keyword').length, 2)
  eq('字符串里的单词不当成函数', ofKind('go', 's := "func hidden"\n', 'function'), [])
  eq('字符串整段上色', ofKind('go', 's := "func hidden"\n', 'string'), ['"func hidden":string'])
  eq('原始字符串跨行', ofKind('go', 's := `hello\nfunc`\n', 'string'), ['`hello:string', 'func`:string'])
  eq('注释和数字', ofKind('go', '// 注释\nn := 1\n', 'comment').concat(ofKind('go', '// 注释\nn := 1\n', 'number')), ['// 注释:comment', '1:number'])
  eq('回车换行后的函数名', syntaxLabels('go', 'func a(\r\n) {}\n').includes('a:function'), true)

  const py = 'class Box:\n    def run(self):\n        print(len("x"))\n        return 1  # note\n'
  eq('Python 类、函数、字符串、数字、注释', picked('python', py, ['type', 'function', 'string', 'number', 'comment']), ['Box:type', 'run:function', 'print:function', 'len:function', '"x":string', '1:number', '# note:comment'])

  const js = 'function fetchUser(id) {\n  const user = new User()\n  return user.save()\n}\nclass User {}\n'
  eq('JavaScript 函数和构造', picked('javascript', js, ['function', 'type']), ['fetchUser:function', 'User:type', 'save:function', 'User:type'])
  eq('if 不是函数', ofKind('javascript', 'if (ok) { call() }\n', 'function'), ['call:function'])

  const tmpl = 'const s = `func ${fetchUser(1)}`\n'
  eq('模板字符串里的函数', picked('javascript', tmpl, ['function', 'string']), [
    '`func :string',
    'fetchUser:function',
    '`:string',
  ])

  const ts = 'interface Row { name: string }\nfunction get(row: Row): Id { return row.name }\n'
  eq('TypeScript 类型名', ofKind('typescript', ts, 'type'), ['Row:type', 'string:type', 'Row:type', 'Id:type'])
  eq('TypeScript 函数名', ofKind('typescript', ts, 'function'), ['get:function'])

  const java = 'public class Box {\n  public Box make() { return new Box(); }\n}\n'
  eq('Java 类和方法', picked('java', java, ['type', 'function']), ['Box:type', 'Box:type', 'make:function', 'Box:type'])

  const cpp = 'class Box {\n  int size() { return 1; }\n};\nvoid run() {}\n'
  eq('C 和 C++ 类型与函数', picked('cpp', cpp, ['type', 'function']), ['Box:type', 'int:type', 'size:function', 'void:type', 'run:function'])

  const cs = 'class Box {\n  public string Name { get; }\n  void Run() {}\n}\n'
  eq('C# 类型与方法', picked('csharp', cs, ['type', 'function']), ['Box:type', 'string:type', 'Name:type', 'void:type', 'Run:function'])

  const php = '<?php\nclass Box {\n  public function run($n) {\n    $this->run();\n    return strlen("x");\n  }\n}\n'
  eq('PHP 类、方法和变量', picked('php', php, ['type', 'function', 'string']), ['Box:type', 'run:function', 'run:function', 'strlen:function', '"x":string'])
  eq('PHP 变量名不单独上色', syntaxLabels('php', php).some((item) => item.startsWith('this:') || item.startsWith('n:')), false)

  const rust = 'struct Box { n: i32 }\nfn run(b: Box) -> i32 {\n    let v = String::new();\n    println!("hi");\n}\n'
  eq('Rust 类型、函数和宏', picked('rust', rust, ['type', 'function', 'string']), ['Box:type', 'i32:type', 'run:function', 'Box:type', 'i32:type', 'String:type', 'new:function', 'println:function', '"hi":string'])

  eq(
    '语义着色编码',
    Array.from(encodeSyntaxSpans([
      { line: 0, start: 1, length: 4, kind: 'keyword' },
      { line: 0, start: 6, length: 3, kind: 'function' },
      { line: 1, start: 0, length: 2, kind: 'type' },
    ])),
    [0, 1, 4, 0, 0, 0, 5, 3, 1, 0, 1, 0, 2, 2, 0],
  )
}
