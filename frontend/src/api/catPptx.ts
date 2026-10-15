import { callService } from './call'
export interface PptPreview { id: string; documentId: string; version: number; state: string; mode: string; url?: string; token?: string; slide: number; error?: string }
export const startPptPreview = (convId: string, relPath: string): Promise<PptPreview> => callService('CatService', 'StartCatPptPreview', { convId, relPath })
export const readPptPreview = (id: string, version: number): Promise<PptPreview> => callService('CatService', 'ReadCatPptPreview', id, version)
export const releasePptPreview = (id: string): Promise<void> => callService('CatService', 'ReleaseCatPptPreview', id)
export const retryPptTool = (): Promise<{ state: string; error?: string }> => callService('CatService', 'GetCatPptTool', true)
