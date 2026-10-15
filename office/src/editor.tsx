// Use GenOffice's real editor entry point. Only the Electron preload is adapted.
import { connectSlidesBridge } from './slides-bridge'

connectSlidesBridge().then(() => import('../vendor/slides/renderer/main')).catch(error => {
  document.getElementById('root')!.textContent = error.message
  parent.postMessage({ type: 'ffc-office-error', message: error.message }, '*')
})
