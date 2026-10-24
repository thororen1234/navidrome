// errorMessage extracts what the server said from an httpClient error. The downloader/Tidal
// endpoints reply with plain-text bodies (http.Error), which react-admin keeps in error.body,
// while error.message is only the HTTP status text ("Bad Request").
export const errorMessage = (error, fallback) => {
  const body = error?.body
  if (typeof body === 'string' && body.trim()) return body.trim()
  if (body?.error) return body.error
  if (body?.message) return body.message
  return fallback
}
