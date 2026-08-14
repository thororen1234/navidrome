import { useState } from 'react'
import { useDispatch } from 'react-redux'
import { Title, useNotify, useTranslate } from 'react-admin'
import {
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  TextField,
  InputAdornment,
  IconButton,
  Typography,
  List,
  ListItem,
  ListItemAvatar,
  ListItemText,
  ListItemSecondaryAction,
  Divider,
  CircularProgress,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PersonIcon from '@material-ui/icons/Person'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import AlbumIcon from '@material-ui/icons/Album'
import { httpClient } from '../dataProvider'
import { baseUrl } from '../utils'
import { setTrack, playTracks } from '../actions'
import DownloadButton from './DownloadButton'

const msToTime = (seconds) => {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

// coverArtUrl points at the same-origin proxy so the browser never needs direct network access
// to TidalSubsonic. Like the audio stream, a plain <img src> can't carry the app's normal auth
// header, so the token rides along as the "jwt" query param instead.
const coverArtUrl = (coverArt, size) =>
  coverArt
    ? baseUrl(
        `/api/tidal/coverArt/${encodeURIComponent(coverArt)}?size=${size}&jwt=${localStorage.getItem('token')}`,
      )
    : undefined

const streamUrl = (id) =>
  baseUrl(`/api/tidal/stream/${id}?jwt=${localStorage.getItem('token')}`)

// toQueueItem shapes a Tidal track into the same "external stream" item Navidrome's radio
// stations use (see radio/helper.jsx): isRadio: true routes it through the player's queue
// without Navidrome's own subsonic stream resolution or playback-reporting, both of which
// assume a local media file id.
const toQueueItem = (track) => ({
  id: track.id,
  name: track.title,
  title: track.title,
  artist: track.artist,
  album: track.album,
  streamUrl: streamUrl(track.id),
  cover: coverArtUrl(track.coverArt, 300),
  isRadio: true,
})

const TrackRow = ({ track, onPlay }) => (
  <ListItem divider>
    <ListItemAvatar>
      <Avatar variant="square" src={coverArtUrl(track.coverArt, 40)}>
        <MusicNoteIcon />
      </Avatar>
    </ListItemAvatar>
    <ListItemText
      primary={track.title}
      secondary={`${track.artist || ''}${track.album ? ' - ' + track.album : ''}${track.duration ? ' · ' + msToTime(track.duration) : ''
        }`}
    />
    <ListItemSecondaryAction>
      <IconButton size="small" onClick={() => onPlay(track)}>
        <PlayArrowIcon fontSize="small" />
      </IconButton>
      <DownloadButton tidalId={track.id} tidalKind="track" />
    </ListItemSecondaryAction>
  </ListItem>
)

const AlbumRow = ({ album, onSelect, onPlay }) => (
  <ListItem button divider onClick={() => onSelect(album)}>
    <ListItemAvatar>
      <Avatar variant="square" src={coverArtUrl(album.coverArt, 40)}>
        <AlbumIcon />
      </Avatar>
    </ListItemAvatar>
    <ListItemText
      primary={album.name}
      secondary={`${album.artist || ''}${album.songCount ? ' · ' + album.songCount + ' tracks' : ''
        }`}
    />
    <ListItemSecondaryAction>
      <IconButton
        size="small"
        onClick={(e) => {
          e.stopPropagation()
          onPlay(album)
        }}
      >
        <PlayArrowIcon fontSize="small" />
      </IconButton>
      <DownloadButton tidalId={album.id} tidalKind="album" />
    </ListItemSecondaryAction>
  </ListItem>
)

const ArtistRow = ({ artist, onSelect }) => (
  <ListItem button divider onClick={() => onSelect(artist)}>
    <ListItemAvatar>
      <Avatar>
        <PersonIcon />
      </Avatar>
    </ListItemAvatar>
    <ListItemText primary={artist.name} />
  </ListItem>
)

const AlbumDetail = ({ album, onPlay, onPlayAll, onBack }) => {
  const translate = useTranslate()
  return (
    <Box mt={2}>
      <Box
        display="flex"
        alignItems="center"
        justifyContent="space-between"
        flexWrap="wrap"
      >
        <Box display="flex" alignItems="center">
          <Avatar
            variant="square"
            src={coverArtUrl(album.coverArt, 160)}
            style={{ width: 80, height: 80, marginRight: 16 }}
          >
            <AlbumIcon />
          </Avatar>
          <Box>
            <Typography variant="h6">{album.name}</Typography>
            <Typography variant="body2" color="textSecondary" gutterBottom>
              {album.artist}
            </Typography>
          </Box>
        </Box>
        <Button
          size="small"
          startIcon={<PlayArrowIcon />}
          onClick={() => onPlayAll(album)}
        >
          {translate('ra.action.export', { _: 'Play all' })}
        </Button>
      </Box>
      <List dense>
        {(album.song || []).map((track) => (
          <TrackRow key={track.id} track={track} onPlay={onPlay} />
        ))}
      </List>
      <Box mt={1}>
        <Button size="small" onClick={onBack}>
          {translate('ra.action.back', { _: 'Back' })}
        </Button>
      </Box>
    </Box>
  )
}

const ArtistDetail = ({ artist, onSelectAlbum, onPlayAlbum, onBack }) => {
  const translate = useTranslate()
  const albums = artist.album || []
  return (
    <Box mt={2}>
      <Box display="flex" alignItems="center" mb={1}>
        <Avatar
          src={coverArtUrl(albums[0]?.coverArt, 160)}
          style={{ width: 56, height: 56, marginRight: 12 }}
        >
          <PersonIcon />
        </Avatar>
        <Typography variant="h6">{artist.name}</Typography>
      </Box>
      {albums.length > 0 ? (
        <List dense>
          {albums.map((al) => (
            <AlbumRow
              key={al.id}
              album={al}
              onSelect={onSelectAlbum}
              onPlay={onPlayAlbum}
            />
          ))}
        </List>
      ) : (
        <Typography variant="body2" color="textSecondary">
          {translate('ra.navigation.no_results', { _: 'No albums found' })}
        </Typography>
      )}
      <Box mt={1}>
        <Button size="small" onClick={onBack}>
          {translate('ra.action.back', { _: 'Back' })}
        </Button>
      </Box>
    </Box>
  )
}

const TidalPage = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const dispatch = useDispatch()
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(null)
  // Navigation stack of { type: 'artist' | 'album', data }, so "Back" from an album opened
  // through an artist returns to that artist rather than to the search results.
  const [nav, setNav] = useState([])
  const current = nav[nav.length - 1]

  const runSearch = () => {
    if (!query.trim()) return
    setLoading(true)
    setNav([])
    httpClient(`/api/tidal/search?q=${encodeURIComponent(query)}`)
      .then(({ json }) => setResults(json))
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
        setResults(null)
      })
      .finally(() => setLoading(false))
  }

  const openArtist = (artist) => {
    httpClient(`/api/tidal/artist/${artist.id}`)
      .then(({ json }) => setNav((n) => [...n, { type: 'artist', data: json }]))
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
      })
  }

  const openAlbum = (album) => {
    httpClient(`/api/tidal/album/${album.id}`)
      .then(({ json }) => setNav((n) => [...n, { type: 'album', data: json }]))
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
      })
  }

  const playTrack = (track) => dispatch(setTrack(toQueueItem(track)))

  const playAlbum = (album) => {
    httpClient(`/api/tidal/album/${album.id}`)
      .then(({ json }) => {
        const tracks = json.song || []
        if (!tracks.length) return
        const data = {}
        tracks.forEach((t) => {
          data[t.id] = toQueueItem(t)
        })
        dispatch(
          playTracks(
            data,
            tracks.map((t) => t.id),
          ),
        )
      })
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
      })
  }

  const goBack = () => setNav((n) => n.slice(0, -1))

  return (
    <Box mt={2}>
      <Title
        title={
          'Navidrome - ' + translate('menu.tidal', { _: 'Tidal Library' })
        }
      />
      <Card>
        <CardContent>
          <TextField
            fullWidth
            variant="outlined"
            placeholder={translate('ra.action.search', { _: 'Search Tidal…' })}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && runSearch()}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={runSearch}>
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
            }}
          />

          {loading && (
            <Box mt={2} textAlign="center">
              <CircularProgress size={24} />
            </Box>
          )}

          {!loading && current?.type === 'artist' && (
            <ArtistDetail
              artist={current.data}
              onSelectAlbum={openAlbum}
              onPlayAlbum={playAlbum}
              onBack={goBack}
            />
          )}

          {!loading && current?.type === 'album' && (
            <AlbumDetail
              album={current.data}
              onPlay={playTrack}
              onPlayAll={playAlbum}
              onBack={goBack}
            />
          )}

          {!loading && !current && results && (
            <Box mt={2}>
              {results.artist?.length > 0 && (
                <>
                  <Typography variant="subtitle1">Artists</Typography>
                  <List dense>
                    {results.artist.map((a) => (
                      <ArtistRow key={a.id} artist={a} onSelect={openArtist} />
                    ))}
                  </List>
                  <Divider />
                </>
              )}
              {results.album?.length > 0 && (
                <>
                  <Typography variant="subtitle1">Albums</Typography>
                  <List dense>
                    {results.album.map((al) => (
                      <AlbumRow
                        key={al.id}
                        album={al}
                        onSelect={openAlbum}
                        onPlay={playAlbum}
                      />
                    ))}
                  </List>
                  <Divider />
                </>
              )}
              {results.song?.length > 0 && (
                <>
                  <Typography variant="subtitle1">Songs</Typography>
                  <List dense>
                    {results.song.map((s) => (
                      <TrackRow key={s.id} track={s} onPlay={playTrack} />
                    ))}
                  </List>
                </>
              )}
              {!results.artist?.length &&
                !results.album?.length &&
                !results.song?.length && (
                  <Typography variant="body2" color="textSecondary">
                    {translate('ra.navigation.no_results', {
                      _: 'No results',
                    })}
                  </Typography>
                )}
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  )
}

export default TidalPage
