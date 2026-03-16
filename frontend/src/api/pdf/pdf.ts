import api from '../index'

export interface PDFInfo {
  name: string
  url: string
}

export const uploadPDFFile = (formData: FormData, onProgress?: (percent: number) => void) => {
  return api.post('/api/uploadPDF', formData, {
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

export const getPDFFiles = () => {
  return api.get('/api/getPDFFiles')
}

export const deletePDFFile = (pdfInfo: PDFInfo) => {
  return api.post('/api/deletePDFFile', pdfInfo)
}
