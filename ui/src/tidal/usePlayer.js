import { useState } from 'react'
import { baseUrl } from '../utils'

// usePlayer drives a single shared <audio> element for in-browser Tidal previews. Streaming goes
// through Navidrome's same-origin proxy (/api/tidal/stream/:id), so it rides the existing
// session auth. The proxy sits behind the same JWT auth as the rest of the native API, but a
// plain <audio src> request can't carry the "X-ND-Authorization" header the app normally sends,
// so the token is passed as the "jwt" query param instead (same trick used by the SSE event
// stream), which server.JWTVerifier also accepts.
export const usePlayer = () => {
  const [queue, setQueue] = useState([])
  const [index, setIndex] = useState(0)

  const playingId = queue[index]
  const hasNext = index + 1 < queue.length

  const play = (id) => {
    setQueue([id])
    setIndex(0)
  }

  const playAll = (ids) => {
    if (!ids || !ids.length) return
    setQueue(ids)
    setIndex(0)
  }

  const stop = () => {
    setQueue([])
    setIndex(0)
  }

  const onEnded = () => {
    if (hasNext) {
      setIndex((i) => i + 1)
    } else {
      stop()
    }
  }

  const src = playingId
    ? baseUrl(
        `/api/tidal/stream/${playingId}?jwt=${localStorage.getItem('token')}`,
      )
    : undefined

  return { playingId, src, play, playAll, stop, onEnded }
}
