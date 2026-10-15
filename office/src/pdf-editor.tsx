import { connectOffice, installPdf } from './file-bridge'
connectOffice().then(cap => {
  installPdf(cap)
  return import('../vendor/apps/pdf/src/renderer/main')
}).catch(error => {
  const message = error instanceof Error ? error.message : String(error)
  document.getElementById('root')!.textContent = message
  parent.postMessage({ type: 'ffc-office-error', message }, '*')
})
