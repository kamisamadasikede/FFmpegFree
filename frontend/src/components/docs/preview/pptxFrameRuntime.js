/* The CLI supplies all slide drawing and typography. This script only controls the viewport. */
(() => {
  const channel = document.documentElement.dataset.channel
  const send = (type, data = {}) => parent.postMessage({ source: 'ffc-ppt-view', channel, type, ...data }, '*')
  let scale = 1, fit = true, page = 1, follow = true, scheduled = false
  const slides = () => [...document.querySelectorAll('.slide-container[data-slide]')]
  const viewport = () => document.querySelector('.main')
  function layout() {
    const list = slides(), first = list[0]?.querySelector('.slide')
    if (!first) return
    const width = first.offsetWidth, height = first.offsetHeight
	if (!width || !height || !document.documentElement.clientWidth) return
    const available = Math.max(40, document.documentElement.clientWidth - 32)
    if (fit) scale = Math.min(2.5, Math.max(.05, available / width))
    list.forEach(container => {
      const wrapper = container.querySelector('.slide-wrapper'), slide = container.querySelector('.slide')
      container.style.width = `${width * scale}px`
      if (wrapper) { wrapper.style.width = `${width * scale}px`; wrapper.style.height = `${height * scale}px` }
      if (slide) { slide.style.transform = `scale(${scale})`; slide.style.transformOrigin = 'top left' }
    })
    send('scale', { scale }); reportPage()
  }
  function reportPage() {
    const list = slides(), main = viewport(); if (!main || !list.length) return
    const top = main.getBoundingClientRect().top
    const nearest = list.reduce((a, b) => Math.abs(a.getBoundingClientRect().top - top) < Math.abs(b.getBoundingClientRect().top - top) ? a : b)
    page = Number(nearest.dataset.slide) || 1
    send('page', { page })
  }
  function go(n) { const list = slides(); page = Math.max(1, Math.min(list.length, Number(n) || 1)); list[page - 1]?.scrollIntoView({ block: 'start' }); send('page', { page }) }
  function update(m) {
    const old = slides().map(n => n.innerHTML), oldScroll = viewport()?.scrollTop || 0
    const parsed = new DOMParser().parseFromString(m.html, 'text/html')
    // Extra defense: inactive markup remains inactive even if a caller bypasses backend sanitization.
    parsed.querySelectorAll('script,iframe,object,embed,base,link,meta,form,input,button,audio,video').forEach(n => n.remove())
    parsed.querySelectorAll('*').forEach(n => [...n.attributes].forEach(a => {
      if (/^on/i.test(a.name) || ['href','srcdoc','action'].includes(a.name) || a.name === 'src' && !/^data:image\//i.test(a.value)) n.removeAttribute(a.name)
    }))
    document.querySelectorAll('style[data-cli]').forEach(n => n.remove())
    parsed.querySelectorAll('style').forEach(n => { n.dataset.cli = '1'; document.head.insertBefore(n, document.getElementById('viewport-style')) })
    document.body.replaceChildren(...parsed.body.childNodes)
    const main = viewport()
    if (main) main.addEventListener('scroll', () => { if (!scheduled) { scheduled = true; requestAnimationFrame(() => { scheduled = false; reportPage() }) } }, { passive: true })
    layout()
	requestAnimationFrame(layout)
    const list = slides(); let changed = Number(m.slide) || 0
    if (!changed) changed = list.findIndex((n, i) => n.innerHTML !== old[i]) + 1
    send('ready', { pages: list.length })
    if (follow && changed) {
      let target
      if (m.scrollTo) { try { target = document.querySelector(m.scrollTo) } catch {} }
      ;(target || list[changed - 1])?.scrollIntoView({ block: 'nearest' })
    } else if (main) main.scrollTop = oldScroll
    reportPage()
  }
  addEventListener('message', e => {
    const m = e.data
    if (e.source !== parent || m?.channel !== channel || m?.source !== 'ffc-ppt-host') return
    if (m.type === 'html') update(m)
    if (m.type === 'scale') { fit = false; scale = Math.min(2.5, Math.max(.05, Number(m.scale) || 1)); layout() }
    if (m.type === 'fit') { fit = true; layout() }
    if (m.type === 'page') go(m.page)
    if (m.type === 'follow') follow = !!m.follow
	if (m.type === 'layout') layout()
  })
  new ResizeObserver(layout).observe(document.documentElement)
  send('boot')
})()
