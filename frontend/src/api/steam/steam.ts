import api from "@/api";
interface VideoInfo {
    name: string
    url: string
    duration: string
    date: string
    steamurl: string
    targetFormat: string
}
export const StopStream = async (row:VideoInfo) => {

    return await  api.post('/api/StopStream',{...row})
}

export const GetStreamingFiles = async () => {

    return await  api.get('/api/GetStreamingFiles')
}