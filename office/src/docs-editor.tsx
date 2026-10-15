import { connectOffice, installDocs } from './file-bridge'
connectOffice().then(cap => {
  installDocs(cap)
  return import('../vendor/apps/docs/src/renderer/main')
}).catch(error => {
  const message = error instanceof Error ? error.message : String(error)
  document.getElementById('root')!.textContent = message
  parent.postMessage({ type: 'ffc-office-error', message }, '*')
})
