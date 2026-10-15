import { connectOffice, installMarkdown } from './file-bridge'
connectOffice().then(cap => {
  installMarkdown(cap)
  return import('../vendor/apps/markdown/src/renderer/main')
}).catch(error => {
  const message = error instanceof Error ? error.message : String(error)
  document.getElementById('root')!.textContent = message
  parent.postMessage({ type: 'ffc-office-error', message }, '*')
})
