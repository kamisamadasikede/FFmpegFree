import { callService } from '@/api/call'

export interface CatBrowserPage {
  url: string
  title: string
  html: string
}

export async function openCatBrowser(url: string): Promise<CatBrowserPage> {
  const raw = await callService<unknown>('CatService', 'OpenCatBrowser', { url })
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    url: typeof r.url === 'string' ? r.url : '',
    title: typeof r.title === 'string' ? r.title : '',
    html: typeof r.html === 'string' ? r.html : '',
  }
}
