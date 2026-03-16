import api from '../index'

export interface ErrorPos {
  line: number
  column: number
}

export interface JsonFormatRequest {
  json: string
  indent?: number
  compact?: boolean
}

export interface JsonFormatResponse {
  formatted: string
  error: string
  errorPos: ErrorPos
}

export interface Difference {
  type: 'added' | 'removed' | 'modified'
  path: string
  oldValue: string
  newValue: string
}

export interface JsonCompareRequest {
  json1: string
  json2: string
}

export interface JsonCompareResponse {
  identical: boolean
  differences: Difference[]
  error: string
  errorPos: ErrorPos
}

export interface JsonValidateRequest {
  json: string
}

export interface JsonValidateResponse {
  valid: boolean
  error: string
  errorPos: ErrorPos
}

export const formatJson = (data: JsonFormatRequest) => {
  return api.post('/api/json/format', data)
}

export const compareJson = (data: JsonCompareRequest) => {
  return api.post('/api/json/compare', data)
}

export const validateJson = (data: JsonValidateRequest) => {
  return api.post('/api/json/validate', data)
}
