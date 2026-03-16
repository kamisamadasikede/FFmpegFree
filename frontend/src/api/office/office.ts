import api from '../index'

export interface OfficeInfo {
  name: string
  url: string
  targetFormat?: string
}

export const uploadOfficeFile = (formData: FormData, onProgress?: (percent: number) => void) => {
  return api.post('/api/uploadOffice', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    onUploadProgress: (progressEvent) => {
      if (onProgress && progressEvent.total) {
        const percent = Math.round(
          (progressEvent.loaded * 100) / progressEvent.total
        )
        onProgress(percent)
      }
    },
  })
}

export const convertOfficeToPDF = (officeInfo: OfficeInfo) => {
  return api.post('/api/convertOfficeToPDF', {
    name: officeInfo.name,
    url: officeInfo.url,
    targetFormat: 'pdf',
  })
}

export const getOfficeFiles = () => {
  return api.get('/api/getOfficeFiles')
}

export const getConvertedPDFiles = () => {
  return api.get('/api/getConvertedPDFiles')
}

export const downloadOfficePDF = async (fileName: string): Promise<Blob> => {
  const response = await api.get<Blob>(
    `/api/downloadOfficePDF?name=${encodeURIComponent(fileName)}`,
    {
      responseType: 'blob',
    }
  )
  return response.data
}

export const deleteOfficeFile = (officeInfo: OfficeInfo) => {
  return api.post('/api/deleteOfficeFile', officeInfo)
}

export const deleteOfficePDF = (officeInfo: OfficeInfo) => {
  return api.post('/api/deleteOfficePDF', officeInfo)
}

export const stopOfficeConversion = (officeInfo: OfficeInfo) => {
  return api.post('/api/stopOfficeConversion', officeInfo)
}
