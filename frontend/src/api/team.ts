import { callService } from './call'
export interface Human { id: string; name: string; role: string; avatar: string; notifyMentions: boolean }
export interface Employee { id: string; name: string; role: string; avatar: string; specialties: string; focus: string; standards: string; knowledge: string; agentKind: string; model: string; effort: string; permissionMode: string; transport: string; enabled: boolean; version: number }
export interface TeamRoom { id: string; name: string; projectId: string; workspace: string; coordinatorId: string; paused: boolean; archived: boolean; rosterVersion: number; updatedAt: number }
export interface TeamMember { id: string; roomId: string; kind: 'human' | 'agent'; profileId: string; name: string; role: string; active: boolean; generation: number }
export interface TeamMessage { id: string; roomId: string; roomSeq: number; revision: number; fromMemberId: string; taskId?: string; runId?: string; replyToId?: string; kind: string; content: string; mentions: string[]; status: string; createdAt: number }
export interface TeamTask { id: string; roomId: string; memberId: string; requesterId: string; parentId?: string; messageId: string; title: string; instruction: string; acceptance: string[]; status: string; revision: number; runId: string; canSteer: boolean; inputMode: string; activity: string; result: string; error: string; required: boolean; dependsOn: string[]; waitFor: string[]; validation: string; createdAt: number; updatedAt: number }
export interface TeamInbox { id: string; roomId: string; messageId: string; taskId: string; recipientId: string; needsReply: boolean; readAt: number; answeredAt: number; notificationStatus: string; createdAt: number }
export interface TeamArtifact { id: string; roomId: string; taskId: string; path: string; version: string; updatedAt: number }
export interface TeamFile { path: string; size: number; modifiedAt: number }
export interface TeamSnapshot { room: TeamRoom; members: TeamMember[]; messages: TeamMessage[]; tasks: TeamTask[]; inbox: TeamInbox[]; artifacts: TeamArtifact[]; eventSeq: number; hasEarlier: boolean }
export interface TeamBootstrap { human: Human; employees: Employee[]; rooms: TeamRoom[]; inbox: TeamInbox[]; modes: Record<string, { id: string; label: string }[]> }
export interface TeamEvent { roomId: string; eventSeq: number; kind: string; data: unknown }
export interface TeamCommand { apiVersion: 1; requestId: string; roomId: string; intent: 'new_task' | 'publish' | 'reply' | 'steer'; content: string; targetMemberIds: string[]; replyToMessageId?: string; targetTaskId?: string; expectedRunId?: string }
export interface TeamDelivery { memberId: string; taskId?: string; status: string; error?: string }
export interface TeamResult { message: TeamMessage; deliveries: TeamDelivery[] }
const invoke = <T>(method: string, ...args: unknown[]): Promise<T> => callService('TeamService', method, ...args)
export const getTeamBootstrap = () => invoke<TeamBootstrap>('GetTeamBootstrap')
export const getTeamRoom = (id: string, before = 0) => invoke<TeamSnapshot>('GetTeamRoom', id, before)
export const getTeamEvents = (id: string, after: number) => invoke<TeamEvent[]>('GetTeamEvents', id, after)
export const saveTeamHuman = (human: Human) => invoke<Human>('SaveTeamHuman', human)
export const saveTeamEmployee = (employee: Employee) => invoke<Employee>('SaveTeamEmployee', employee)
export const createTeamRoom = (req: { name: string; projectId: string; employeeIds: string[]; coordinatorId: string }) => invoke<TeamRoom>('CreateTeamRoom', req)
export const updateTeamRoom = (room: TeamRoom) => invoke<TeamRoom>('UpdateTeamRoom', room)
export const setTeamMember = (room: string, employee: string, active: boolean) => invoke<void>('SetTeamMember', room, employee, active)
export const sendTeamMessage = (command: TeamCommand) => invoke<TeamResult>('SendTeamMessage', command)
export const stopTeamTask = (id: string) => invoke<void>('StopTeamTask', id)
export const continueTeamTask = (id: string, content: string) => invoke<void>('ContinueTeamTask', id, content)
export const markTeamRead = (room: string, ids: string[]) => invoke<void>('MarkTeamMessagesRead', room, ids)
export const listTeamFiles = (room: string) => invoke<{ files: TeamFile[]; truncated: boolean }>('ListTeamFiles', room)
export const startTeamPreview = (room: string, path: string) => invoke<import('./catPptx').PptPreview>('StartTeamPreview', room, path)
export const releaseTeamPreview = (id: string) => invoke<void>('ReleaseTeamPreview', id)
