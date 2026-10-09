// requestJSON calls the API and always resolves to [error, result], never throws:
// [null, pick(data)] on success, [message, null] for API errors ({error: true}),
// network failures and non-JSON responses (e.g. a proxy error page).
export async function requestJSON<T>(url: string, options: any, pick: (data: any) => T): Promise<[string | null, T | null]> {
  try {
    const response = await fetch(url, options)
    const body = await response.json()
    if (body.error) {
      return [body.message || `request failed with status ${response.status}`, null]
    }
    return [null, pick(body.data)]
  } catch (e) {
    return [e instanceof Error ? e.message : String(e), null]
  }
}
