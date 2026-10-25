import { useEffect, useRef, useState } from 'react'
import { useDispatch } from 'react-redux'
import { useHistory, useLocation } from 'react-router-dom'
import { Title, useNotify, useTranslate } from 'react-admin'
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import { httpClient } from '../dataProvider'
import { playTrack, playTrackList } from './playback'
import {
  AlbumGrid,
  AlbumHeader,
  ArtistGrid,
  ArtistHeader,
  SectionTitle,
  TrackTable,
} from './TidalViews'

// Tidal returns every single and EP as an "album"; splitting by track count keeps an artist's
// real albums from drowning in dozens of one-track releases. An unknown count counts as an
// album.
const MIN_ALBUM_TRACKS = 7
const isFullAlbum = (album) =>
  !album.songCount || album.songCount >= MIN_ALBUM_TRACKS
const byYearDesc = (a, b) => (b.year || 0) - (a.year || 0)

// The current view lives in the URL (?q=, ?artist=, ?album=), so browser Back/Forward and
// reloads work like the rest of the app, instead of a page-local navigation stack.
const useTidalLocation = () => {
  const location = useLocation()
  const history = useHistory()
  const params = new URLSearchParams(location.search)
  const go = (next) => {
    const p = new URLSearchParams()
    Object.entries(next).forEach(([k, v]) => v && p.set(k, v))
    history.push({ pathname: location.pathname, search: `?${p}` })
  }
  return {
    q: params.get('q') || '',
    artistId: params.get('artist'),
    albumId: params.get('album'),
    go,
    back: () => history.goBack(),
  }
}

// useTidalFetch loads url whenever it changes. data is only returned for the current url: a
// late response for a page the user has left is dropped, and the previous view's data never
// renders under the new view while it loads.
const useTidalFetch = (url) => {
  const notify = useNotify()
  // Read through a ref so the effect only re-runs for a new url, never for a new notify.
  const notifyRef = useRef(notify)
  notifyRef.current = notify
  const [state, setState] = useState({ url: null, data: null, failed: false })
  useEffect(() => {
    if (!url) return undefined
    let active = true
    httpClient(url)
      .then(({ json }) => {
        if (active) setState({ url, data: json, failed: false })
      })
      .catch((error) => {
        if (!active) return
        notifyRef.current(error.message || 'ra.page.error', {
          type: 'warning',
        })
        setState({ url, data: null, failed: true })
      })
    return () => {
      active = false
    }
  }, [url])
  const current = state.url === url
  return {
    loading: !!url && !current,
    data: current ? state.data : null,
    failed: current && state.failed,
  }
}

const SearchResults = ({ results, onOpenArtist, onOpenAlbum, onPlayAlbum }) => {
  const translate = useTranslate()
  const dispatch = useDispatch()
  const artists = results.artist || []
  const albums = results.album || []
  const songs = results.song || []
  if (!artists.length && !albums.length && !songs.length) {
    return (
      <Box mt={3}>
        <Typography variant="body2" color="textSecondary">
          {translate('ra.navigation.no_results')}
        </Typography>
      </Box>
    )
  }
  return (
    <>
      {artists.length > 0 && (
        <>
          <SectionTitle>
            {translate('resources.tidal.sections.artists')}
          </SectionTitle>
          <ArtistGrid artists={artists} onOpen={onOpenArtist} />
        </>
      )}
      {albums.length > 0 && (
        <>
          <SectionTitle>
            {translate('resources.tidal.sections.albums')}
          </SectionTitle>
          <AlbumGrid
            albums={albums}
            showArtist
            onOpen={onOpenAlbum}
            onPlay={onPlayAlbum}
          />
        </>
      )}
      {songs.length > 0 && (
        <>
          <SectionTitle>
            {translate('resources.tidal.sections.songs')}
          </SectionTitle>
          <TrackTable
            tracks={songs}
            showArtist
            showAlbum
            onPlay={(t) => playTrack(dispatch, t)}
          />
        </>
      )}
    </>
  )
}

const ArtistView = ({ artist, onOpenAlbum, onPlayAlbum }) => {
  const translate = useTranslate()
  const all = [...(artist.album || [])].sort(byYearDesc)
  const albums = all.filter(isFullAlbum)
  const singles = all.filter((a) => !isFullAlbum(a))
  return (
    <>
      <ArtistHeader artist={artist} albums={all} />
      {albums.length > 0 && (
        <>
          <SectionTitle>
            {translate('resources.tidal.sections.albums')}
          </SectionTitle>
          <AlbumGrid
            albums={albums}
            onOpen={onOpenAlbum}
            onPlay={onPlayAlbum}
          />
        </>
      )}
      {singles.length > 0 && (
        <>
          <SectionTitle>
            {translate('resources.tidal.sections.singles')}
          </SectionTitle>
          <AlbumGrid
            albums={singles}
            onOpen={onOpenAlbum}
            onPlay={onPlayAlbum}
          />
        </>
      )}
      {!all.length && (
        <Box mt={3}>
          <Typography variant="body2" color="textSecondary">
            {translate('resources.tidal.messages.noAlbums')}
          </Typography>
        </Box>
      )}
    </>
  )
}

const AlbumView = ({ album, onOpenArtist }) => {
  const dispatch = useDispatch()
  const songs = album.song || []
  return (
    <>
      <AlbumHeader album={album} onOpenArtist={onOpenArtist} />
      <Box mt={2}>
        <TrackTable
          tracks={songs}
          showArtist={songs.some((s) => s.artist && s.artist !== album.artist)}
          onPlay={(t) => playTrackList(dispatch, songs, t.id)}
        />
      </Box>
    </>
  )
}

const TidalPage = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const dispatch = useDispatch()
  const { q, artistId, albumId, go, back } = useTidalLocation()
  const [query, setQuery] = useState(q)
  useEffect(() => setQuery(q), [q])

  const view = albumId ? 'album' : artistId ? 'artist' : q ? 'search' : null
  const url =
    view === 'album'
      ? `/api/tidal/album/${encodeURIComponent(albumId)}`
      : view === 'artist'
        ? `/api/tidal/artist/${encodeURIComponent(artistId)}`
        : view === 'search'
          ? `/api/tidal/search?q=${encodeURIComponent(q)}`
          : null
  const { loading, data, failed } = useTidalFetch(url)

  const runSearch = () => {
    const trimmed = query.trim()
    if (trimmed) go({ q: trimmed })
  }
  // Keep the search in the URL while browsing, so the search box still shows it.
  const openArtist = (artist) => go({ q, artist: artist.id })
  const openAlbum = (album) => go({ q, album: album.id })

  // Album tiles in grids carry no track list, so fetch it before playing.
  const playAlbum = (album) =>
    httpClient(`/api/tidal/album/${encodeURIComponent(album.id)}`)
      .then(({ json }) => playTrackList(dispatch, json.song || []))
      .catch((error) =>
        notify(error.message || 'ra.page.error', { type: 'warning' }),
      )

  return (
    <Box mt={2} mb={4}>
      <Title
        title={'Navidrome - ' + translate('menu.tidal', { _: 'Tidal Library' })}
      />
      <Box display="flex" alignItems="center" style={{ gap: 8 }}>
        {(view === 'artist' || view === 'album') && (
          <IconButton onClick={back} aria-label={translate('ra.action.back')}>
            <ArrowBackIcon />
          </IconButton>
        )}
        <TextField
          fullWidth
          variant="outlined"
          size="small"
          placeholder={translate('resources.tidal.messages.searchPlaceholder')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && runSearch()}
          InputProps={{
            endAdornment: (
              <InputAdornment position="end">
                <IconButton size="small" onClick={runSearch}>
                  <SearchIcon />
                </IconButton>
              </InputAdornment>
            ),
          }}
        />
      </Box>

      {loading && (
        <Box mt={4} textAlign="center">
          <CircularProgress size={28} />
        </Box>
      )}

      {!loading && data && (
        <Box mt={2}>
          {view === 'search' && (
            <SearchResults
              results={data}
              onOpenArtist={openArtist}
              onOpenAlbum={openAlbum}
              onPlayAlbum={playAlbum}
            />
          )}
          {view === 'artist' && (
            <ArtistView
              artist={data}
              onOpenAlbum={openAlbum}
              onPlayAlbum={playAlbum}
            />
          )}
          {view === 'album' && (
            <AlbumView album={data} onOpenArtist={openArtist} />
          )}
        </Box>
      )}

      {failed && (
        <Box mt={3}>
          <Button size="small" onClick={back}>
            {translate('ra.action.back')}
          </Button>
        </Box>
      )}
    </Box>
  )
}

export default TidalPage
