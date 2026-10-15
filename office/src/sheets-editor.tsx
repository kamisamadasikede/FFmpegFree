import { connectOffice, installSheets } from './file-bridge'
connectOffice().then(cap => {
  installSheets(cap)
  return import('../vendor/apps/sheets/src/renderer/main')
}).catch(error => {
  const message = error instanceof Error ? error.message : String(error)
  document.getElementById('root')!.textContent = message
  parent.postMessage({ type: 'ffc-office-error', message }, '*')
})
