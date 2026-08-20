import { request } from './http'

export type SIPTestResult = {
  status: string
  detail?: string
}

function sipTest(path: string, deviceAccessID: string): Promise<SIPTestResult> {
  return request(path, { method: 'POST', body: JSON.stringify({ device_access_id: deviceAccessID }) })
}

export function sipRegister(deviceAccessID: string): Promise<SIPTestResult> {
  return sipTest('/api/v1/test/sip/register', deviceAccessID)
}

export function sipKeepAlive(deviceAccessID: string): Promise<SIPTestResult> {
  return sipTest('/api/v1/test/sip/keepalive', deviceAccessID)
}

export function sipUnregister(deviceAccessID: string): Promise<SIPTestResult> {
  return sipTest('/api/v1/test/sip/unregister', deviceAccessID)
}
