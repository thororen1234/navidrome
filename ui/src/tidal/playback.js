import { addTracks, playNext, playTracks, setTrack } from '../actions'
import { tidalCoverArtUrl, tidalStreamUrl } from './urls'

// Queue keys must not look like integers: playTracks keys its data by id, and JS orders
// integer-like object keys numerically - so raw (numeric) Tidal ids would play an album sorted
// by track id instead of in track order.
const queueKey = (id) => `tidal:${id}`

// toQueueItem shapes a Tidal track into the same "external stream" item Navidrome's radio
// stations use (see radio/helper.jsx): isRadio: true routes it through the player's queue
// without Navidrome's own subsonic stream resolution or playback-reporting, both of which
// assume a local media file id. tidalId lets the player reducer rebuild streamUrl with a fresh
// token when a persisted queue is restored.
const toQueueItem = (track) => ({
  id: queueKey(track.id),
  tidalId: track.id,
  name: track.title,
  title: track.title,
  artist: track.artist,
  album: track.album,
  duration: track.duration,
  streamUrl: tidalStreamUrl(track.id),
  cover: tidalCoverArtUrl(track.coverArt, 300),
  isRadio: true,
})

const toQueueData = (tracks) => {
  const data = {}
  tracks.forEach((t) => {
    data[queueKey(t.id)] = toQueueItem(t)
  })
  return [data, tracks.map((t) => queueKey(t.id))]
}

const shuffled = (list) => {
  const out = [...list]
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[out[i], out[j]] = [out[j], out[i]]
  }
  return out
}

export const playTrack = (dispatch, track) =>
  dispatch(setTrack(toQueueItem(track)))

// playTrackList replaces the queue with tracks, starting at selectedId when given.
export const playTrackList = (dispatch, tracks, selectedId) => {
  if (!tracks?.length) return
  const [data, ids] = toQueueData(tracks)
  dispatch(playTracks(data, ids, selectedId && queueKey(selectedId)))
}

export const shuffleTrackList = (dispatch, tracks) =>
  playTrackList(dispatch, shuffled(tracks || []))

export const playTrackListNext = (dispatch, tracks) => {
  if (!tracks?.length) return
  dispatch(playNext(...toQueueData(tracks)))
}

export const addTrackListToQueue = (dispatch, tracks) => {
  if (!tracks?.length) return
  dispatch(addTracks(...toQueueData(tracks)))
}
