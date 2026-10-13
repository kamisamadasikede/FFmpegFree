import { AppError, callService } from '@/api/call'
import { catSim } from '@/views/cat/catDevSim'

export interface CatBrowserPage {
  url: string
  title: string
  html: string
}

export async function openCatBrowser(url: string): Promise<CatBrowserPage> {
  // 仅 vite dev + ?cat_sim=browsertimeout：走查超时失败态（正式包 catSim 恒空）
  if (catSim.has('browsertimeout')) {
    throw new AppError('IO_ERROR', '打开超时，请检查网络或换网址。')
  }
  const raw = await callService<unknown>('CatService', 'OpenCatBrowser', { url })
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    url: typeof r.url === 'string' ? r.url : '',
    title: typeof r.title === 'string' ? r.title : '',
    html: typeof r.html === 'string' ? r.html : '',
  }
}
