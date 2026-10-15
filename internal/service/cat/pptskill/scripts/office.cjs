#!/usr/bin/env node
'use strict'
const fs = require('node:fs')
const crypto = require('node:crypto')
const readline = require('node:readline')
const argv = process.argv.slice(2)
const flags = {}
const action = argv[0]?.startsWith('--') ? '' : argv.shift()
for (let i = 0; i < argv.length; i++) {
  const name = argv[i]
  if (!name.startsWith('--')) throw new Error('Arguments must use named flags')
  flags[name.slice(2)] = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[++i] : true
}
const endpoint = flags.endpoint || process.env.FFC_OFFICE_ENDPOINT
const token = flags.token || process.env.FFC_OFFICE_TOKEN
async function call(request) {
  if (!endpoint || !token) throw new Error('ffc office host is not attached; run the Agent inside ffc')
  const parsed = new URL(endpoint)
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1') throw new Error('Office endpoint must be loopback')
  const response = await fetch(endpoint, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
    body: JSON.stringify(request), signal: AbortSignal.timeout(60000),
  })
  const text = await response.text()
  let result
  try { result = JSON.parse(text) } catch { throw new Error('Office host returned an invalid response') }
  if (!response.ok || result.error) throw new Error(result.error?.message || 'Office request failed')
  return result
}
const tools = [
  { name: 'office_open', description: 'Open/create a native PPTX in the ffc live canvas before generating. Return revision.', properties: { create: { type: 'boolean' } } },
  { name: 'office_read', description: 'Read native slide/element IDs, text, geometry and current revision.' },
  { name: 'office_page', description: 'Build and immediately show one native 1280x720 page spec (text/shapes/images). Use current revision and unique requestId.', mutate: true, properties: { page: { type: 'object' }, index: { type: 'integer', minimum: 0 }, replace: { type: 'boolean' } }, required: ['page'] },
  { name: 'office_apply', description: 'Apply an atomic batch of native GenOffice ops. Geometry is EMU. Read guide for schemas.', mutate: true, properties: { ops: { type: 'array', minItems: 1, maxItems: 50, items: { type: 'object' } }, dryRun: { type: 'boolean' } }, required: ['ops'] },
  { name: 'office_save', description: 'Save the live PPTX atomically. Verify savedRevision equals revision. copyPath saves a separate workspace copy on conflict.', properties: { expectedRevision: { type: 'integer' }, copyPath: { type: 'string' } } },
  { name: 'office_audit', description: 'Check rendered pages for text overflow and layout findings before final delivery.' },
  { name: 'office_guide', description: 'Read exact native operation schemas. Groups: text, element, slide, insert, table, deck.', properties: { group: { type: 'string' } } },
].map(tool => ({ name: tool.name, description: tool.description, inputSchema: {
  type: 'object', properties: { path: { type: 'string', description: 'Workspace-relative .pptx path' }, ...(tool.mutate ? { expectedRevision: { type: 'integer' }, requestId: { type: 'string' } } : {}), ...tool.properties },
  required: ['path', ...(tool.mutate ? ['expectedRevision', 'requestId'] : []), ...(tool.required || [])],
} }))
async function mcp(message) {
  if (message.id === undefined) return
  if (message.method === 'initialize') return { protocolVersion: message.params?.protocolVersion || '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: 'ffc-office', version: '1.0.0' } }
  if (message.method === 'ping') return {}
  if (message.method === 'tools/list') return { tools }
  if (message.method === 'tools/call') {
    try {
      const name = message.params?.name
      if (!tools.some(tool => tool.name === name)) throw new Error('Unknown office tool')
      const result = await call({ ...message.params.arguments, action: name.slice('office_'.length) })
      return { content: [{ type: 'text', text: JSON.stringify(result) }] }
    } catch (e) { return { isError: true, content: [{ type: 'text', text: e.message }] } }
  }
  throw new Error('Unsupported MCP method')
}
if (flags.mcp) {
  const lines = readline.createInterface({ input: process.stdin })
  lines.on('line', line => {
    let message
    try { message = JSON.parse(line) } catch { return }
    if (message.id === undefined) return
    mcp(message).then(result => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: message.id, result }) + '\n'),
      error => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: message.id, error: { code: -32601, message: error.message } }) + '\n'))
  })
} else {
  ;(async () => {
    if (!action || !flags.path) throw new Error('Use open/read/page/apply/save/guide/audit/undo/redo --path FILE.pptx [--input JSON] [--revision N] [--request-id ID]')
    const request = { action, path: flags.path }
    if (flags.create) request.create = true
    if (flags.revision !== undefined) request.expectedRevision = Number(flags.revision)
    if (flags.group) request.group = flags.group
    if (flags.copy) request.copyPath = flags.copy
    if (flags.index !== undefined) request.index = Number(flags.index)
    if (flags.replace) request.replace = true
    if (['page', 'apply', 'undo', 'redo'].includes(action)) {
      if (!Number.isInteger(request.expectedRevision)) throw new Error('Read the document first and pass --revision N')
      request.requestId = flags['request-id'] || crypto.randomUUID()
    }
    if (flags.input) {
      const content = JSON.parse(fs.readFileSync(flags.input, 'utf8').replace(/^\uFEFF/, ''))
      if (action === 'page') request.page = content
      else if (action === 'apply') request.ops = Array.isArray(content) ? content : content.ops
    }
    process.stdout.write(JSON.stringify(await call(request)) + '\n')
  })().catch(error => { process.stderr.write(JSON.stringify({ error: { message: error.message } }) + '\n'); process.exitCode = 1 })
}
