<template>
  <div class="json-tools-container">
    <el-tabs v-model="activeTab" type="border-card" class="json-tabs">
      <el-tab-pane label="JSON 格式化" name="format">
        <div class="format-section">
          <el-row :gutter="20">
            <el-col :span="7">
              <div class="panel-header">
                <span>输入原始 JSON</span>
                <div class="header-actions">
                  <el-button size="small" type="danger" plain @click="clearFormatInput">清空</el-button>
                </div>
              </div>
              <div class="editor-outer editor-container" ref="formatInputEditorRef"></div>
              <div v-if="formatError" class="error-info">
                <el-alert 
                  :title="`JSON 语法错误: 第 ${formatErrorPos.line} 行, 第 ${formatErrorPos.column} 列`" 
                  :description="formatError" 
                  type="error" 
                  :closable="false" 
                  show-icon 
                />
              </div>
            </el-col>
            <el-col :span="17">
              <div class="panel-header res-header">
                <span>格式化结果</span>
                <div class="header-actions">
                  <el-button size="small" @click="copyFormatOutput">复制</el-button>
                  <el-button size="small" type="primary" @click="handleFormat">格式化</el-button>
                </div>
              </div>
              <div class="editor-outer">
                <div class="editor-container" ref="formatOutputEditorRef"></div>
              </div>
            </el-col>
          </el-row>
          
          <div class="format-options">
            <el-checkbox v-model="compactMode">压缩单行</el-checkbox>
            <span class="label">缩进空格:</span>
            <el-input-number v-model="indentSize" :min="2" :max="8" :step="2" size="small" />
            <span class="tips">自动格式化：停止输入 800ms 后自动格式化</span>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="JSON 比对" name="compare">
        <div class="compare-section">
          <el-row :gutter="20">
            <el-col :span="7">
              <div class="panel-header">
                <span>JSON 1 (原始)</span>
                <div class="header-actions">
                  <el-button size="small" type="danger" plain @click="clearCompareInput1">清空</el-button>
                </div>
              </div>
              <div class="editor-outer">
                <div class="editor-container" ref="compareInput1EditorRef"></div>
              </div>
              <div v-if="compareError1" class="error-info">
                <el-alert 
                  :title="`JSON 语法错误: 第 ${compareErrorPos1.line} 行`" 
                  :description="compareError1" 
                  type="error" 
                  :closable="false" 
                  show-icon 
                />
              </div>
            </el-col>
            <el-col :span="17">
              <div class="panel-header">
                <span>JSON 2 (对比)</span>
                <div class="header-actions">
                  <el-button size="small" type="danger" plain @click="clearCompareInput2">清空</el-button>
                </div>
              </div>
              <div class="editor-outer">
                <div class="editor-container" ref="compareInput2EditorRef"></div>
              </div>
              <div v-if="compareError2" class="error-info">
                <el-alert 
                  :title="`JSON 语法错误: 第 ${compareErrorPos2.line} 行`" 
                  :description="compareError2" 
                  type="error" 
                  :closable="false" 
                  show-icon 
                />
              </div>
            </el-col>
          </el-row>
          
          <div class="compare-actions">
            <el-button type="primary" size="large" @click="handleCompare" :disabled="!canCompare">
              比对 JSON
            </el-button>
          </div>
          
          <div v-if="compareResult" class="compare-result fade-in">
            <el-alert
              v-if="compareResult.identical"
              title="✅ 两个 JSON 完全相同"
              type="success"
              :closable="false"
            />
            <div v-else-if="compareResult.differences && compareResult.differences.length > 0">
              <el-alert
                :title="`⚠️ 发现 ${compareResult.differences.length} 处差异`"
                type="warning"
                :closable="false"
              />
              <el-table :data="compareResult.differences" style="width: 100%; margin-top: 20px;">
                <el-table-column prop="type" label="类型" width="100">
                  <template #default="scope">
                    <el-tag :type="getDiffTypeTag(scope.row.type)">
                      {{ getDiffTypeLabel(scope.row.type) }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="path" label="路径" width="200" />
                <el-table-column label="旧值" width="220">
                  <template #default="scope">
                    <span class="old-value">{{ scope.row.oldValue || '-' }}</span>
                  </template>
                </el-table-column>
                <el-table-column label="新值">
                  <template #default="scope">
                    <span class="new-value">{{ scope.row.newValue || '-' }}</span>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import * as monaco from 'monaco-editor'
import { formatJson, compareJson, JsonFormatResponse, JsonCompareResponse } from '@/api/json/json'

const activeTab = ref('format')

// 格式化相关
const formatInput = ref('')
const formatError = ref('')
const formatErrorPos = ref({ line: 1, column: 1 })
const compactMode = ref(false)
const indentSize = ref(4)
let formatTimer: ReturnType<typeof setTimeout> | null = null
let lastFormattedValue = ''

// 比对相关
const compareInput1 = ref('')
const compareInput2 = ref('')
const compareError1 = ref('')
const compareError2 = ref('')
const compareErrorPos1 = ref({ line: 1, column: 1 })
const compareErrorPos2 = ref({ line: 1, column: 1 })
const compareResult = ref<JsonCompareResponse | null>(null)
let compareTimer1: ReturnType<typeof setTimeout> | null = null
let compareTimer2: ReturnType<typeof setTimeout> | null = null
let lastCompareValue1 = ''
let lastCompareValue2 = ''

// 编辑器引用
const formatInputEditorRef = ref<HTMLElement | null>(null)
const formatOutputEditorRef = ref<HTMLElement | null>(null)
const compareInput1EditorRef = ref<HTMLElement | null>(null)
const compareInput2EditorRef = ref<HTMLElement | null>(null)

// 编辑器实例
let formatInputEditor: monaco.editor.IStandaloneCodeEditor | null = null
let formatOutputEditor: monaco.editor.IStandaloneCodeEditor | null = null
let compareInput1Editor: monaco.editor.IStandaloneCodeEditor | null = null
let compareInput2Editor: monaco.editor.IStandaloneCodeEditor | null = null

let compareDecorations1: string[] = []
let compareDecorations2: string[] = []
let isSyncingScroll = false

const createJsonEditor = (container: HTMLElement, readOnly: boolean = false, onChange?: (value: string) => void) => {
  const editor = monaco.editor.create(container, {
    value: '',
    language: 'json',
    theme: 'vs',
    readOnly: readOnly,
    automaticLayout: true,
    tabSize: indentSize.value,
    insertSpaces: true,
    detectIndentation: false,
    fontSize: 14,
    fontFamily: "'Cascadia Code', 'Consolas', 'Monaco', monospace",
    fixedOverflowWidgets: true,
    renderLineHighlight: 'line',
    folding: true,
    lineNumbers: 'on',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    bracketPairColorization: { enabled: true },
    guides: {
      indentation: true,
      bracketPairs: true,
      highlightActiveIndentation: true
    },
    scrollbar: {
      verticalScrollbarSize: 8,
      horizontalScrollbarSize: 8
    }
  })
  
  if (onChange) {
    editor.onDidChangeModelContent(() => {
      onChange(editor.getValue())
    })
  }
  
  return editor
}

// 验证 JSON 并标记错误
const validateAndMarkErrors = (editor: monaco.editor.IStandaloneCodeEditor | null, errorRef: any, errorPosRef: any) => {
  if (!editor) return
  
  const value = editor.getValue()
  const model = editor.getModel()
  if (!model) return
  
  // 清除之前的标记
  monaco.editor.setModelMarkers(model, 'json-validator', [])
  
  if (!value.trim()) {
    errorRef.value = ''
    errorPosRef.value = { line: 1, column: 1 }
    return
  }
  
  try {
    JSON.parse(value)
    errorRef.value = ''
    errorPosRef.value = { line: 1, column: 1 }
  } catch (e: any) {
    errorRef.value = e.message
    const pos = parseErrorPosition(value, e.message)
    errorPosRef.value = pos
    
    // 添加错误标记 - 红色波浪线
    const match = e.message.match(/position (\d+)/i) || e.message.match(/offset (\d+)/i)
    let startLine = 1
    let startColumn = 1
    
    if (match) {
      const offset = parseInt(match[1])
      const pos = calculateLineColumn(value, offset)
      startLine = pos.line
      startColumn = pos.column
    }
    
    // 找到该行的结束位置
    const lineContent = model.getLineContent(startLine)
    const endColumn = lineContent.length + 1
    
    monaco.editor.setModelMarkers(model, 'json-validator', [{
      startLineNumber: startLine,
      startColumn: startColumn,
      endLineNumber: startLine,
      endColumn: endColumn,
      message: e.message,
      severity: monaco.MarkerSeverity.Error
    }])
  }
}

const parseErrorPosition = (jsonStr: string, errMsg: string): { line: number; column: number } => {
  const lineColMatch = errMsg.match(/line (\d+) column (\d+)/i)
  if (lineColMatch) {
    return {
      line: parseInt(lineColMatch[1]),
      column: parseInt(lineColMatch[2])
    }
  }
  
  const posMatch = errMsg.match(/position (\d+)/i)
  if (posMatch) {
    const pos = parseInt(posMatch[1])
    return calculateLineColumn(jsonStr, pos)
  }
  
  const offsetMatch = errMsg.match(/offset (\d+)/i)
  if (offsetMatch) {
    const offset = parseInt(offsetMatch[1])
    return calculateLineColumn(jsonStr, offset)
  }
  
  return { line: 1, column: 1 }
}

const calculateLineColumn = (str: string, offset: number): { line: number; column: number } => {
  if (offset > str.length) offset = str.length
  
  let line = 1
  let column = 1
  
  for (let i = 0; i < offset; i++) {
    if (str[i] === '\n') {
      line++
      column = 1
    } else {
      column++
    }
  }
  
  return { line, column }
}

// 监听缩进变化
watch(indentSize, (val) => {
  const editors = [formatInputEditor, formatOutputEditor, compareInput1Editor, compareInput2Editor]
  editors.forEach(editor => {
    editor?.getModel()?.updateOptions({ tabSize: val })
  })
})

watch([compactMode, indentSize], () => {
  if (formatInput.value.trim() && !formatError.value) {
    triggerAutoFormat(formatInput.value)
  }
})

watch([compareInput1, compareInput2], () => {
  compareResult.value = null
})

onMounted(async () => {
  await nextTick()
  
  if (formatInputEditorRef.value) {
    formatInputEditor = createJsonEditor(formatInputEditorRef.value, false, (value) => {
      formatInput.value = value
      validateAndMarkErrors(formatInputEditor, formatError, formatErrorPos)
      triggerAutoFormat(value)
    })
  }
  
  if (formatOutputEditorRef.value) {
    formatOutputEditor = createJsonEditor(formatOutputEditorRef.value, true)
  }
  
  if (compareInput1EditorRef.value) {
    compareInput1Editor = createJsonEditor(compareInput1EditorRef.value, false, (value) => {
      compareInput1.value = value
      validateAndMarkErrors(compareInput1Editor, compareError1, compareErrorPos1)
      triggerCompareAutoFormat(compareInput1Editor, value, true)
    })
  }
  
  if (compareInput2EditorRef.value) {
    compareInput2Editor = createJsonEditor(compareInput2EditorRef.value, false, (value) => {
      compareInput2.value = value
      validateAndMarkErrors(compareInput2Editor, compareError2, compareErrorPos2)
      triggerCompareAutoFormat(compareInput2Editor, value, false)
    })
  }
  
  if (compareInput1Editor && compareInput2Editor) {
    compareInput1Editor.onDidScrollChange((e) => {
      if (!isSyncingScroll && e.scrollTopChanged) {
        isSyncingScroll = true
        compareInput2Editor?.setScrollTop(e.scrollTop)
        compareInput2Editor?.setScrollLeft(e.scrollLeft)
        setTimeout(() => { isSyncingScroll = false }, 50)
      }
    })
    compareInput2Editor.onDidScrollChange((e) => {
      if (!isSyncingScroll && e.scrollTopChanged) {
        isSyncingScroll = true
        compareInput1Editor?.setScrollTop(e.scrollTop)
        compareInput1Editor?.setScrollLeft(e.scrollLeft)
        setTimeout(() => { isSyncingScroll = false }, 50)
      }
    })
  }
})

const canCompare = computed(() => {
  return compareInput1.value.trim() !== '' && 
         compareInput2.value.trim() !== '' && 
         !compareError1.value && 
         !compareError2.value
})

const doFormat = async (isAuto: boolean = false) => {
  if (!formatInput.value.trim()) {
    if (isAuto) return
    ElMessage.warning('请输入 JSON 字符串')
    return
  }
  
  // 先验证
  if (formatError.value) {
    if (isAuto) return
    ElMessage.error('JSON 格式有误，请先修正错误')
    return
  }
  
  try {
    const response = await formatJson({
      json: formatInput.value,
      indent: indentSize.value,
      compact: compactMode.value
    })
    
    const data = response.data as any
    if (data.code === 200) {
      const result = data.data as JsonFormatResponse
      if (result.error) {
        formatError.value = result.error
        formatErrorPos.value = result.errorPos
        if (!isAuto) ElMessage.error('JSON 格式有误')
      } else {
        formatError.value = ''
        if (formatOutputEditor?.getValue() !== result.formatted) {
          formatOutputEditor?.setValue(result.formatted)
        }
        
        setTimeout(() => {
          formatOutputEditor?.getAction('editor.action.formatDocument')?.run()
          formatOutputEditor?.getAction('editor.unfoldAll')?.run()
        }, 50)
        
        if (!isAuto) ElMessage.success('格式化成功')
      }
    }
  } catch (err) {
    try {
      const obj = JSON.parse(formatInput.value)
      formatOutputEditor?.setValue(JSON.stringify(obj, null, indentSize.value))
      if (!isAuto) ElMessage.info('后端接口异常，已通过前端对齐')
    } catch (e) {
      if (!isAuto) ElMessage.error('无效的 JSON')
    }
  }
}

const handleFormat = () => {
  doFormat(false)
}

const triggerAutoFormat = (currentValue: string) => {
  if (currentValue === lastFormattedValue) {
    return
  }
  if (formatTimer) {
    clearTimeout(formatTimer)
  }
  formatTimer = setTimeout(() => {
    doFormat(true)
    lastFormattedValue = formatOutputEditor?.getValue() || ''
  }, 800)
}

const triggerCompareAutoFormat = (editor: monaco.editor.IStandaloneCodeEditor | null, value: string, isFirst: boolean) => {
  const lastValue = isFirst ? lastCompareValue1 : lastCompareValue2
  const setLastValue = (v: string) => {
    if (isFirst) lastCompareValue1 = v
    else lastCompareValue2 = v
  }
  
  if (value === lastValue) {
    return
  }
  
  const timer = isFirst ? compareTimer1 : compareTimer2
  const setTimer = (t: ReturnType<typeof setTimeout> | null) => {
    if (isFirst) compareTimer1 = t
    else compareTimer2 = t
  }
  
  if (timer) {
    clearTimeout(timer)
  }
  
  const newTimer = setTimeout(() => {
    if (!value.trim() || !editor) return
    if (isFirst ? compareError1.value : compareError2.value) return
    
    try {
      const obj = JSON.parse(value)
      editor.setValue(JSON.stringify(obj, null, indentSize.value))
      setLastValue(editor.getValue())
      compareResult.value = null
      setTimeout(() => {
        editor.getAction('editor.unfoldAll')?.run()
      }, 50)
    } catch (e) {
      // 格式错误时不自动格式化
    }
  }, 800)
  
  setTimer(newTimer)
}

const handleCompare = async () => {
  if (!canCompare.value) {
    ElMessage.warning('请确保两个 JSON 都没有语法错误')
    return
  }
  
  try {
    const response = await compareJson({
      json1: compareInput1.value,
      json2: compareInput2.value
    })
    
    const data = response.data as any
    if (data.code === 200) {
      const result = data.data as JsonCompareResponse
      compareResult.value = result
      
      clearCompareDecorations()
      
      if (result.error) {
        ElMessage.error(result.error)
      } else if (result.identical) {
        ElMessage.success('两个 JSON 完全相同')
      } else {
        highlightDifferences(result.differences)
        ElMessage.warning(`发现 ${result.differences.length} 处差异`)
      }
    }
  } catch (error: any) {
    ElMessage.error('比对失败: ' + (error.message || '未知错误'))
  }
}

const clearCompareDecorations = () => {
  if (compareInput1Editor) {
    compareDecorations1 = compareInput1Editor.deltaDecorations(compareDecorations1, [])
  }
  if (compareInput2Editor) {
    compareDecorations2 = compareInput2Editor.deltaDecorations(compareDecorations2, [])
  }
}

const highlightDifferences = (differences: any[]) => {
  const decorations1: any[] = []
  const decorations2: any[] = []
  
  differences.forEach(diff => {
    const pos = findJsonValuePosition(diff.path, diff.oldValue, diff.newValue, diff.type)
    if (pos) {
      if (diff.type === 'removed') {
        decorations1.push({
          range: new monaco.Range(pos.oldLine, pos.oldStartCol, pos.oldLine, pos.oldEndCol),
          options: {
            isWholeLine: false,
            className: 'diff-highlight-removed',
            hoverMessage: { value: `删除: ${diff.oldValue}` }
          }
        })
      } else if (diff.type === 'added') {
        decorations2.push({
          range: new monaco.Range(pos.newLine, pos.newStartCol, pos.newLine, pos.newEndCol),
          options: {
            isWholeLine: false,
            className: 'diff-highlight-added',
            hoverMessage: { value: `新增: ${diff.newValue}` }
          }
        })
      } else if (diff.type === 'modified') {
        decorations1.push({
          range: new monaco.Range(pos.oldLine, pos.oldStartCol, pos.oldLine, pos.oldEndCol),
          options: {
            isWholeLine: false,
            className: 'diff-highlight-removed',
            hoverMessage: { value: `旧值: ${diff.oldValue}` }
          }
        })
        decorations2.push({
          range: new monaco.Range(pos.newLine, pos.newStartCol, pos.newLine, pos.newEndCol),
          options: {
            isWholeLine: false,
            className: 'diff-highlight-added',
            hoverMessage: { value: `新值: ${diff.newValue}` }
          }
        })
      }
    }
  })
  
  if (compareInput1Editor && decorations1.length > 0) {
    compareDecorations1 = compareInput1Editor.deltaDecorations([], decorations1)
  }
  if (compareInput2Editor && decorations2.length > 0) {
    compareDecorations2 = compareInput2Editor.deltaDecorations([], decorations2)
  }
}

const findJsonValuePosition = (
  path: string, 
  oldValue: string, 
  newValue: string, 
  diffType: string
): { oldLine: number, oldStartCol: number, oldEndCol: number, newLine: number, newStartCol: number, newEndCol: number } | null => {
  const model1 = compareInput1Editor?.getModel()
  const model2 = compareInput2Editor?.getModel()
  
  const content1 = model1?.getValue() || ''
  const content2 = model2?.getValue() || ''
  
  const keys = path.split('.').filter(k => k && !k.match(/^\d+$/))
  
  if (keys.length === 0) {
    const simplePos1 = findValueInLine(content1, oldValue)
    const simplePos2 = findValueInLine(content2, newValue)
    return {
      oldLine: simplePos1?.line || 1,
      oldStartCol: simplePos1?.startCol || 1,
      oldEndCol: simplePos1?.endCol || 10,
      newLine: simplePos2?.line || 1,
      newStartCol: simplePos2?.startCol || 1,
      newEndCol: simplePos2?.endCol || 10
    }
  }
  
  const searchKey = keys[keys.length - 1]
  
  const pos1 = findKeyValuePosition(content1, searchKey, oldValue)
  const pos2 = findKeyValuePosition(content2, searchKey, newValue)
  
  return {
    oldLine: pos1?.line || 1,
    oldStartCol: pos1?.startCol || 1,
    oldEndCol: pos1?.endCol || 10,
    newLine: pos2?.line || 1,
    newStartCol: pos2?.startCol || 1,
    newEndCol: pos2?.endCol || 10
  }
}

const findKeyValuePosition = (content: string, key: string, value: string): { line: number, startCol: number, endCol: number } | null => {
  const lines = content.split('\n')
  
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const keyIndex = line.indexOf(`"${key}"`)
    if (keyIndex !== -1) {
      const valueInfo = extractValueRange(line, keyIndex + key.length + 2)
      if (valueInfo) {
        return {
          line: i + 1,
          startCol: keyIndex + 1,
          endCol: keyIndex + key.length + 2 + valueInfo.length
        }
      }
    }
  }
  
  return findValueInLine(content, value)
}

const findValueInLine = (content: string, value: string): { line: number, startCol: number, endCol: number } | null => {
  if (!value) return null
  
  const lines = content.split('\n')
  const searchValue = value.length > 50 ? value.substring(0, 50) : value
  
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const valueIndex = line.indexOf(searchValue)
    if (valueIndex !== -1) {
      return {
        line: i + 1,
        startCol: valueIndex + 1,
        endCol: valueIndex + value.length + 1
      }
    }
  }
  
  return null
}

const extractValueRange = (line: string, afterKeyPos: number): { value: string, length: number } | null => {
  const rest = line.substring(afterKeyPos).trim()
  if (!rest) return null
  
  if (rest.startsWith(':')) {
    const afterColon = rest.substring(1).trim()
    
    if (afterColon.startsWith('"')) {
      const endQuote = findEndQuote(afterColon.substring(1))
      if (endQuote >= 0) {
        return { value: afterColon.substring(0, endQuote + 2), length: afterColon.substring(0, endQuote + 2).length + 1 }
      }
    } else if (afterColon.startsWith('{') || afterColon.startsWith('[')) {
      const endBracket = findMatchingBracket(afterColon)
      if (endBracket > 0) {
        return { value: afterColon.substring(0, endBracket + 1), length: endBracket + 1 }
      }
    } else {
      const numMatch = afterColon.match(/^[\d.eE+-]+/)
      if (numMatch) {
        return { value: numMatch[0], length: numMatch[0].length }
      }
      const boolMatch = afterColon.match(/^(true|false|null)/)
      if (boolMatch) {
        return { value: boolMatch[0], length: boolMatch[0].length }
      }
    }
  }
  
  return null
}

const findEndQuote = (str: string): number => {
  for (let i = 0; i < str.length; i++) {
    if (str[i] === '"' && (i === 0 || str[i - 1] !== '\\')) {
      return i
    }
  }
  return -1
}

const findMatchingBracket = (str: string): number => {
  let count = 0
  for (let i = 0; i < str.length; i++) {
    if (str[i] === '{' || str[i] === '[') count++
    else if (str[i] === '}' || str[i] === ']') {
      count--
      if (count === 0) return i
    }
  }
  return -1
}

const clearFormatInput = () => {
  formatInputEditor?.setValue('')
  formatOutputEditor?.setValue('')
  formatError.value = ''
  formatErrorPos.value = { line: 1, column: 1 }
  
  const model = formatInputEditor?.getModel()
  if (model) {
    monaco.editor.setModelMarkers(model, 'json-validator', [])
  }
}

const clearCompareInput1 = () => {
  compareInput1Editor?.setValue('')
  compareError1.value = ''
  compareErrorPos1.value = { line: 1, column: 1 }
  compareResult.value = null
  
  const model = compareInput1Editor?.getModel()
  if (model) {
    monaco.editor.setModelMarkers(model, 'json-validator', [])
  }
}

const clearCompareInput2 = () => {
  compareInput2Editor?.setValue('')
  compareError2.value = ''
  compareErrorPos2.value = { line: 1, column: 1 }
  compareResult.value = null
  
  const model = compareInput2Editor?.getModel()
  if (model) {
    monaco.editor.setModelMarkers(model, 'json-validator', [])
  }
}

const copyFormatOutput = () => {
  const val = formatOutputEditor?.getValue()
  if (val) {
    navigator.clipboard.writeText(val)
    ElMessage.success('复制成功')
  } else {
    ElMessage.warning('暂无可复制内容')
  }
}

const getDiffTypeTag = (type: string): string => {
  switch (type) {
    case 'added': return 'success'
    case 'removed': return 'danger'
    case 'modified': return 'warning'
    default: return 'info'
  }
}

const getDiffTypeLabel = (type: string): string => {
  switch (type) {
    case 'added': return '新增'
    case 'removed': return '删除'
    case 'modified': return '修改'
    default: return type
  }
}

onBeforeUnmount(() => {
  if (formatTimer) {
    clearTimeout(formatTimer)
  }
  if (compareTimer1) {
    clearTimeout(compareTimer1)
  }
  if (compareTimer2) {
    clearTimeout(compareTimer2)
  }
  formatInputEditor?.dispose()
  formatOutputEditor?.dispose()
  compareInput1Editor?.dispose()
  compareInput2Editor?.dispose()
})
</script>

<style scoped>
.json-tools-container {
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.json-tabs {
  flex: 1;
}

.format-section,
.compare-section {
  background: var(--surface);
  border: 1px solid var(--border-soft);
  border-radius: var(--panel-radius);
  padding: 12px;
  box-shadow: var(--shadow-2);
  animation: fadeIn 0.25s ease;
}

.editor-outer {
  width: 100%;
  border: 1px solid var(--border-soft);
  border-radius: 0 0 14px 14px;
  overflow: hidden;
  text-align: left;
  background: #fff;
}

.editor-container {
  width: 100%;
  height: min(62vh, 680px);
  background: #fff;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 15px;
  background: var(--surface-muted);
  color: var(--text-primary);
  font-weight: bold;
  border-radius: 14px 14px 0 0;
  border: 1px solid var(--border-soft);
  border-bottom: none;
}

.res-header {
  background: rgba(37, 99, 235, 0.08);
}

.format-options {
  margin-top: 14px;
  padding: 12px;
  background: var(--surface);
  border-radius: 14px;
  display: flex;
  align-items: center;
  gap: 15px;
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-2);
}

.label {
  font-size: 14px;
  color: var(--text-muted);
}

.tips {
  font-size: 12px;
  color: var(--text-soft);
  margin-left: auto;
}

.error-info {
  margin-top: 10px;
}

.compare-actions {
  display: flex;
  justify-content: center;
  margin-top: 18px;
}

.compare-actions :deep(.el-button--primary) {
  padding: 15px 40px;
  font-size: 16px;
}

.compare-result {
  margin-top: 20px;
}

.old-value {
  color: #dc2626;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
}

.new-value {
  color: #16a34a;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
}

:deep(.monaco-editor) {
  text-align: left !important;
}
:deep(.view-lines) {
  text-align: left !important;
}

:deep(.el-table th) {
  background: var(--surface-muted);
  color: var(--text-muted);
}

:deep(.diff-highlight-removed) {
  background: rgba(220, 38, 38, 0.2) !important;
  border: 1px solid rgba(220, 38, 38, 0.4);
}

:deep(.diff-highlight-added) {
  background: rgba(22, 163, 74, 0.2) !important;
  border: 1px solid rgba(22, 163, 74, 0.4);
}

.fade-in {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@media (max-width: 1200px) {
  .editor-container {
    height: 420px;
  }

  .format-options {
    flex-wrap: wrap;
    justify-content: flex-start;
  }

  .tips {
    width: 100%;
    margin-left: 0;
  }
}
</style>
