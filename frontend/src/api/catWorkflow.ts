import { callService } from './call'
import { hasWailsBackend } from '@/services/wails'
import type { SendCatMessageRequest } from './cat'
export interface FileChange { path: string; status: string; added: number; deleted: number; binary: boolean; diff?: string; truncated?: boolean }
export interface TurnChanges { convId: string; turnId: string; messageId?: string; finishedAt?: number; createdAt: number; fileCount: number; added: number; deleted: number; files: FileChange[]; finished: boolean; truncated: boolean }
export interface QueuedMessage { id: string; request: SendCatMessageRequest; createdAt: number }
export interface MessageQueue { revision?: number; convId: string; items: QueuedMessage[]; paused: boolean; error?: string }
export interface FileRevision { revision: string; size: number; stable: boolean }
export interface DocumentPreview { previewId: string; kind: string; state: string; name: string; ext: string; url?: string; rawUrl?: string; sizeBytes: number; reason?: string; error?: { message: string } }
export const emptyQueue = (convId: string): MessageQueue => ({ convId, items: [], paused: false })
export const getQueue = (convId: string): Promise<MessageQueue> => hasWailsBackend() ? callService('CatService', 'GetCatMessageQueue', convId) : Promise.resolve(emptyQueue(convId))
export const queueMessage = (request: SendCatMessageRequest): Promise<MessageQueue> => callService('CatService', 'QueueCatMessage', request)
export const updateQueue = (convId: string, action: 'pause' | 'resume' | 'edit' | 'delete' | 'guide', id = '', content = ''): Promise<MessageQueue> => callService('CatService', 'UpdateCatMessageQueue', { convId, action, id, content })
export const getChanges = (convId: string): Promise<TurnChanges[]> => hasWailsBackend() ? callService('CatService', 'GetCatTurnChanges', convId) : Promise.resolve([])
export const statFile = (convId: string, relPath: string): Promise<FileRevision> => callService('CatService', 'StatCatFile', { convId, relPath })
export const previewDocument = (convId: string, relPath: string): Promise<DocumentPreview> => callService('CatService', 'PreviewCatDocument', { convId, relPath })
export const releaseDocument = (previewId: string): Promise<void> => callService('CatService', 'ReleaseCatDocument', previewId)
export const openWorkspaceFile = (convId: string, relPath: string): Promise<void> => callService('CatService', 'OpenCatWorkspaceFile', { convId, relPath })

export const getChangeDetail = (convId: string, turnId: string): Promise<TurnChanges> => callService('CatService', 'GetCatTurnChangeDetail', convId, turnId)
