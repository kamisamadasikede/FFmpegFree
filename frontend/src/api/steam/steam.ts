import api from "@/api";
interface VideoInfo {
    name: string
    url: string
    duration: string
    date: string
    steamurl: string
    streamId?: string
    targetFormat: string
    archiveEnabled?: boolean
    segmentSeconds?: number
    relayTargets?: string[]
}
export const StopStream = async (row:VideoInfo) => {

    return await  api.post('/api/live/stream/stop',{...row})
}

export const GetStreamingFiles = async () => {

    return await  api.get('/api/live/stream/list')
}
