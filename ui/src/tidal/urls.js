import { baseUrl } from '../utils'

// Tidal media goes through Navidrome's same-origin proxy, so the browser never needs direct
// network access to TidalSubsonic. A plain <img>/<audio> src can't carry the app's normal auth
// header, so the token rides along as the "jwt" query param instead (same trick as the SSE
// event stream). The token is read at call time: URLs built from a stale token stop working
// once it expires, which is why persisted queue items are rebuilt via tidalStreamUrl on load.
const jwt = () => localStorage.getItem('token')

export const tidalCoverArtUrl = (coverArt, size) =>
  coverArt
    ? baseUrl(
        `/api/tidal/coverArt/${encodeURIComponent(coverArt)}?size=${size}&jwt=${jwt()}`,
      )
    : undefined

export const tidalStreamUrl = (id) =>
  baseUrl(`/api/tidal/stream/${encodeURIComponent(id)}?jwt=${jwt()}`)

// tidalArtistImageUrl prefers the bridge's coverArt id (proxied), falling back to the
// OpenSubsonic artistImageUrl when it is a plain public http(s) image URL.
export const tidalArtistImageUrl = (artist, size) => {
  if (!artist) return undefined
  if (artist.coverArt) return tidalCoverArtUrl(artist.coverArt, size)
  if (/^https?:\/\//i.test(artist.artistImageUrl || '')) {
    return artist.artistImageUrl
  }
  return undefined
}
