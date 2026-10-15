import { connectOffice, installHtml } from './file-bridge'
connectOffice().then(cap => {
  installHtml(cap)
  return import('../vendor/apps/html/src/renderer/main')
}).catch(error => {
  const message = error instanceof Error ? error.message : String(error)
  document.getElementById('root')!.textContent = message
  parent.postMessage({ type: 'ffc-office-error', message }, '*')
})
