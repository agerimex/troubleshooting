// requestJSON calls the API and always resolves to [error, result], never throws:
// [null, pick(data)] on success, [message, null] for non-2xx statuses, API
// errors ({error: true}), network failures and non-JSON responses (e.g. a proxy
// error page).
export async function requestJSON<T>(url: string, options: any, pick: (data: any) => T): Promise<[string | null, T | null]> {
  try {
    const response = await fetch(url, options)
    let body: any = null
    try {
      body = await response.json()
    } catch {
      // not JSON; reported below with the status
    }
    if (!response.ok || body === null || body.error) {
      const reason = body?.message || (response.ok ? 'invalid response' : `request failed with status ${response.status}`)
      return [reason, null]
    }
    return [null, pick(body.data)]
  } catch (e) {
    return [e instanceof Error ? e.message : String(e), null]
  }
}
