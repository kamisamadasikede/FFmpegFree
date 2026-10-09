<template>
  <div class="rich">
    <p v-if="empty" class="note">这个文件是空的。</p>
    <p v-else-if="failed" class="note">这个文件没能显示出来。</p>
    <img v-else-if="kind === 'image' && url" class="pic" :src="url" alt="" />
    <video v-else-if="kind === 'media' && url && mime.startsWith('video/')" class="media" controls :src="url" />
    <audio v-else-if="kind === 'media' && url" class="media" controls :src="url" />
    <iframe v-else-if="kind === 'pdf' && url" class="frame" :title="name" :src="url" />
    <div v-else-if="kind === 'docx'" class="scroll">
      <div ref="docxStyle" />
      <div ref="docxHost" class="docx" />
    </div>
    <div v-else-if="kind === 'xlsx'" class="scroll">
      <div v-if="sheets.length > 1" class="tabs">
        <button v-for="n in sheets" :key="n" type="button" class="tab" :class="{ on: n === sheet }" @click="pickSheet(n)">{{ n }}</button>
      </div>
      <p v-if="!rows.length" class="note">这个表格是空的。</p>
      <table v-else class="grid">
        <tbody>
          <tr v-for="(row, i) in rows" :key="i">
            <td v-for="(cell, j) in row" :key="j">{{ cell }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="sheetCut" class="note">只显示前 200 行、30 列。</p>
    </div>
    <div v-else-if="kind === 'pptx'" class="scroll">
      <p v-if="!slides.length" class="note">这份演示文稿里没有可显示的文字。</p>
      <article v-for="(s, i) in slides" :key="i" class="slide">
        <h3>{{ s.title }}</h3>
        <p v-if="!s.lines.length" class="muted">这一页没有可显示的文字。</p>
        <p v-for="(line, j) in s.lines" :key="j">{{ line }}</p>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { WorkBook } from 'xlsx'

const props = defineProps<{ kind: string; mime: string; b64: string; name: string }>()

const failed = ref(false)
const empty = ref(false)
const url = ref('')
const sheets = ref<string[]>([])
const sheet = ref('')
const rows = ref<string[][]>([])
const sheetCut = ref(false)
const slides = ref<{ title: string; lines: string[] }[]>([])
const docxHost = ref<HTMLElement | null>(null)
const docxStyle = ref<HTMLElement | null>(null)
let objectUrl = ''
let token = 0
let book: WorkBook | null = null
let xlsxMod: typeof import('xlsx') | null = null

function bytesOf(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

function revoke() {
  if (objectUrl) URL.revokeObjectURL(objectUrl)
  objectUrl = ''
  url.value = ''
}

function blobUrl(bytes: Uint8Array, mime: string): string {
  const blob = new Blob([bytes.buffer as ArrayBuffer], { type: mime || 'application/octet-stream' })
  objectUrl = URL.createObjectURL(blob)
  return objectUrl
}

onMounted(() => {
  void render()
})

watch(
  () => [props.kind, props.b64, props.mime] as const,
  () => {
    void render()
  },
)

onBeforeUnmount(() => {
  token++
  revoke()
})

async function render() {
  const my = ++token
  failed.value = false
  empty.value = false
  sheets.value = []
  rows.value = []
  slides.value = []
  sheetCut.value = false
  book = null
  revoke()
  if (!props.b64) {
    empty.value = true
    return
  }
  let bytes: Uint8Array
  try {
    bytes = bytesOf(props.b64)
  } catch {
    failed.value = true
    return
  }
  if (my !== token) return
  if (props.kind === 'image' || props.kind === 'media' || props.kind === 'pdf') {
    url.value = blobUrl(bytes, props.mime)
    return
  }
  if (props.kind === 'docx') {
    await renderDocx(bytes, my)
    return
  }
  if (props.kind === 'xlsx') {
    await renderSheet(bytes, my)
    return
  }
  if (props.kind === 'pptx') await renderSlides(bytes, my)
}

async function renderDocx(bytes: Uint8Array, my: number) {
  await nextTick()
  const host = docxHost.value
  const style = docxStyle.value
  if (!host || my !== token) return
  host.replaceChildren()
  if (style) style.replaceChildren()
  try {
    const { renderAsync } = await import('docx-preview')
    if (my !== token || !docxHost.value) return
    await renderAsync(bytes, docxHost.value, docxStyle.value ?? undefined, {
      inWrapper: false,
      ignoreLastRenderedPageBreak: true,
      renderHeaders: true,
      renderFooters: true,
      useBase64URL: true,
      experimental: false,
    })
    if (my !== token || !docxHost.value) return
    docxHost.value.querySelectorAll('a[href]').forEach((a) => a.removeAttribute('href'))
  } catch {
    if (my === token) failed.value = true
  }
}

async function renderSheet(bytes: Uint8Array, my: number) {
  try {
    const XLSX = await import('xlsx')
    if (my !== token) return
    xlsxMod = XLSX
    book = XLSX.read(bytes, { type: 'array' })
    sheets.value = book.SheetNames.slice()
    sheet.value = book.SheetNames[0] ?? ''
    fillSheet()
  } catch {
    if (my === token) failed.value = true
  }
}

function pickSheet(name: string) {
  sheet.value = name
  fillSheet()
}

function fillSheet() {
  if (!xlsxMod || !book || !sheet.value) {
    rows.value = []
    return
  }
  const grid = xlsxMod.utils.sheet_to_json<unknown[]>(book.Sheets[sheet.value] ?? {}, { header: 1, raw: false, defval: '' })
  sheetCut.value = grid.length > 200 || grid.some((row) => Array.isArray(row) && row.length > 30)
  rows.value = grid.slice(0, 200).map((row) => (Array.isArray(row) ? row : []).slice(0, 30).map((cell) => (cell == null ? '' : String(cell))))
}

function xmlPlain(s: string): string {
  return s
    .replace(/&#x([0-9a-fA-F]+);/g, (_, h: string) => String.fromCodePoint(parseInt(h, 16)))
    .replace(/&#(\d+);/g, (_, n: string) => String.fromCodePoint(Number(n)))
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, '&')
}

function slideNum(path: string): number {
  const m = /slide(\d+)\.xml$/i.exec(path)
  return m ? Number(m[1]) : 0
}

async function renderSlides(bytes: Uint8Array, my: number) {
  try {
    const JSZip = (await import('jszip')).default
    const zip = await JSZip.loadAsync(bytes)
    if (my !== token) return
    const names = Object.keys(zip.files)
      .filter((n) => /^ppt\/slides\/slide\d+\.xml$/i.test(n))
      .sort((a, b) => slideNum(a) - slideNum(b))
      .slice(0, 40)
    const out: { title: string; lines: string[] }[] = []
    for (const path of names) {
      const xml = await zip.files[path].async('string')
      if (my !== token) return
      const lines: string[] = []
      const re = /<a:t[^>]*>([\s\S]*?)<\/a:t>/g
      let m: RegExpExecArray | null
      while ((m = re.exec(xml))) {
        const text = xmlPlain(m[1]).replace(/\s+/g, ' ').trim()
        if (text) lines.push(text)
      }
      out.push({ title: `第 ${out.length + 1} 页`, lines })
    }
    slides.value = out
  } catch {
    if (my === token) failed.value = true
  }
}
</script>

<style scoped>
.rich,
.scroll {
  flex: 1;
  min-height: 0;
  min-width: 0;
}
.rich {
  display: flex;
  flex-direction: column;
}
.scroll {
  overflow: auto;
  padding: 12px;
}
.note,
.muted {
  margin: 0;
  padding: 24px 16px;
  text-align: center;
  color: var(--ff-text-3);
  font-size: 13px;
}
.slide .muted {
  padding: 0;
  text-align: left;
}
.pic,
.media,
.frame {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: none;
  background: var(--ff-bg-app);
  object-fit: contain;
}
.media {
  flex: none;
  margin: auto;
  padding: 16px;
}
.tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 8px;
}
.tab {
  border: none;
  background: transparent;
  color: var(--ff-text-2);
  border-radius: 6px;
  padding: 4px 8px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.tab.on {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.grid {
  border-collapse: collapse;
  font-size: 12px;
  color: var(--ff-text-1);
}
.grid td {
  border: 1px solid var(--ff-border);
  padding: 4px 8px;
  white-space: nowrap;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.slide {
  margin: 0 0 12px;
  padding: 12px;
  border: 1px solid var(--ff-border);
  border-radius: 8px;
}
.slide h3 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
}
.slide p {
  margin: 0 0 4px;
  font-size: 13px;
  line-height: 1.5;
}
.docx :deep(section.docx) {
  box-shadow: none;
  margin: 0 0 12px;
  max-width: 100%;
}
</style>
