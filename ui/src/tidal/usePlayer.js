import { useState } from 'react'
import { baseUrl } from '../utils'

// usePlayer drives a single shared <audio> element for in-browser Tidal previews. Streaming goes
// through Navidrome's same-origin proxy (/api/tidal/stream/:id), so it rides the existing
// session auth and needs no separate Tidal credentials client-side.
export const usePlayer = () => {
  const [playingId, setPlayingId] = useState(null)

  const play = (id) => setPlayingId(id)
  const stop = () => setPlayingId(null)

  const src = playingId ? baseUrl(`/api/tidal/stream/${playingId}`) : undefined

  return { playingId, src, play, stop }
}
