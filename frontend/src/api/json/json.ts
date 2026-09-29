// JSON 工具：直接调用 Go 端 JsonService（Wails 绑定），不经过任何 HTTP 接口。
import { Format, Compare, Validate } from '../../../wailsjs/go/app/JsonService'
import { jsontool } from '../../../wailsjs/go/models'
import { call } from '../call'

export type ErrorPos = jsontool.ErrorPos
export type Difference = jsontool.Difference
export type JsonFormatRequest = { json: string; indent?: number; compact?: boolean }
export type JsonFormatResponse = jsontool.FormatResponse
export type JsonCompareRequest = { json1: string; json2: string }
export type JsonCompareResponse = jsontool.CompareResponse
export type JsonValidateRequest = { json: string }
export type JsonValidateResponse = jsontool.ValidateResponse

export const formatJson = (req: JsonFormatRequest) =>
  call(Format(jsontool.FormatRequest.createFrom({ indent: 0, compact: false, ...req })))

export const compareJson = (req: JsonCompareRequest) =>
  call(Compare(jsontool.CompareRequest.createFrom(req)))

export const validateJson = (req: JsonValidateRequest) =>
  call(Validate(jsontool.ValidateRequest.createFrom(req)))
