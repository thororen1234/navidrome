import { useRef, useState } from 'react'
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
import { setTrack, playTracks } from '../actions'
import DownloadButton from './DownloadButton'
import { tidalArtistImageUrl, tidalCoverArtUrl, tidalStreamUrl } from './urls'

const secondsToTime = (seconds) => {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

const coverArtUrl = tidalCoverArtUrl

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
  cover: coverArtUrl(track.coverArt, 300),
  isRadio: true,
})

const playTrackList = (dispatch, tracks, selectedId) => {
  if (!tracks.length) return
  const data = {}
  tracks.forEach((t) => {
    data[queueKey(t.id)] = toQueueItem(t)
  })
  dispatch(
    playTracks(
      data,
      tracks.map((t) => queueKey(t.id)),
      selectedId && queueKey(selectedId),
    ),
  )
}

const TrackRow = ({ track, onPlay }) => (
  <ListItem divider>
    <ListItemAvatar>
      <Avatar variant="square" src={coverArtUrl(track.coverArt, 80)}>
        <MusicNoteIcon />
      </Avatar>
    </ListItemAvatar>
    <ListItemText
      primary={track.title}
      secondary={[track.artist, track.album, secondsToTime(track.duration)]
        .filter(Boolean)
        .join(' · ')}
    />
    <ListItemSecondaryAction>
      <IconButton size="small" onClick={() => onPlay(track)}>
        <PlayArrowIcon fontSize="small" />
      </IconButton>
      <DownloadButton tidalId={track.id} tidalKind="track" />
    </ListItemSecondaryAction>
  </ListItem>
)

const AlbumRow = ({ album, onSelect, onPlay }) => {
  const translate = useTranslate()
  return (
    <ListItem button divider onClick={() => onSelect(album)}>
      <ListItemAvatar>
        <Avatar variant="square" src={coverArtUrl(album.coverArt, 80)}>
          <AlbumIcon />
        </Avatar>
      </ListItemAvatar>
      <ListItemText
        primary={album.name}
        secondary={[
          album.artist,
          album.year,
          album.songCount &&
            translate('resources.tidal.messages.trackCount', {
              smart_count: album.songCount,
            }),
        ]
          .filter(Boolean)
          .join(' · ')}
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
}

const ArtistRow = ({ artist, onSelect }) => {
  const translate = useTranslate()
  return (
    <ListItem button divider onClick={() => onSelect(artist)}>
      <ListItemAvatar>
        <Avatar src={tidalArtistImageUrl(artist, 80)} alt={artist.name}>
          <PersonIcon />
        </Avatar>
      </ListItemAvatar>
      <ListItemText
        primary={artist.name}
        secondary={
          artist.albumCount
            ? translate('resources.tidal.messages.albumCount', {
                smart_count: artist.albumCount,
              })
            : undefined
        }
      />
    </ListItem>
  )
}

const BackButton = ({ onBack }) => {
  const translate = useTranslate()
  return (
    <Box mt={1}>
      <Button size="small" onClick={onBack}>
        {translate('ra.action.back')}
      </Button>
    </Box>
  )
}

const AlbumDetail = ({ album, onPlayTrack, onPlayAll, onBack }) => {
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
              {[album.artist, album.year].filter(Boolean).join(' · ')}
            </Typography>
          </Box>
        </Box>
        <Box display="flex" alignItems="center">
          <Button
            size="small"
            startIcon={<PlayArrowIcon />}
            onClick={() => onPlayAll(album)}
          >
            {translate('resources.tidal.actions.playAll')}
          </Button>
          <DownloadButton tidalId={album.id} tidalKind="album" />
        </Box>
      </Box>
      <List dense>
        {(album.song || []).map((track) => (
          <TrackRow
            key={track.id}
            track={track}
            onPlay={(t) => onPlayTrack(album, t)}
          />
        ))}
      </List>
      <BackButton onBack={onBack} />
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
          src={
            tidalArtistImageUrl(artist, 160) ||
            coverArtUrl(albums[0]?.coverArt, 160)
          }
          alt={artist.name}
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
          {translate('resources.tidal.messages.noAlbums')}
        </Typography>
      )}
      <BackButton onBack={onBack} />
    </Box>
  )
}

const Section = ({ title, children }) => (
  <>
    <Typography variant="subtitle1">{title}</Typography>
    <List dense>{children}</List>
    <Divider />
  </>
)

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
  // Only the latest request may update the page: a slow response for an earlier click must not
  // replace (or push on top of) what the user navigated to since.
  const requestSeq = useRef(0)

  const onError = (error) =>
    notify(error.message || 'ra.page.error', { type: 'warning' })

  const load = (url, onLoaded) => {
    const seq = ++requestSeq.current
    setLoading(true)
    return httpClient(url)
      .then(({ json }) => {
        if (seq === requestSeq.current) onLoaded(json)
      })
      .catch((error) => {
        if (seq === requestSeq.current) onError(error)
      })
      .finally(() => {
        if (seq === requestSeq.current) setLoading(false)
      })
  }

  const runSearch = () => {
    if (!query.trim()) return
    setNav([])
    load(`/api/tidal/search?q=${encodeURIComponent(query.trim())}`, setResults)
  }

  const openArtist = (artist) =>
    load(`/api/tidal/artist/${encodeURIComponent(artist.id)}`, (json) =>
      setNav((n) => [...n, { type: 'artist', data: json }]),
    )

  const openAlbum = (album) =>
    load(`/api/tidal/album/${encodeURIComponent(album.id)}`, (json) =>
      setNav((n) => [...n, { type: 'album', data: json }]),
    )

  const playTrack = (track) => dispatch(setTrack(toQueueItem(track)))

  // Playing a track from an album's track list queues the whole album from that track on, like
  // the regular album view does.
  const playAlbumFrom = (album, track) =>
    playTrackList(dispatch, album.song || [], track.id)

  // Search/artist album rows carry no track list, so fetch it; the album detail view already
  // has one.
  const playAlbum = (album) => {
    if (album.song?.length) {
      playTrackList(dispatch, album.song)
      return
    }
    httpClient(`/api/tidal/album/${encodeURIComponent(album.id)}`)
      .then(({ json }) => playTrackList(dispatch, json.song || []))
      .catch(onError)
  }

  const goBack = () => {
    requestSeq.current++ // drop any in-flight navigation
    setLoading(false)
    setNav((n) => n.slice(0, -1))
  }

  return (
    <Box mt={2}>
      <Title
        title={'Navidrome - ' + translate('menu.tidal', { _: 'Tidal Library' })}
      />
      <Card>
        <CardContent>
          <TextField
            fullWidth
            variant="outlined"
            placeholder={translate(
              'resources.tidal.messages.searchPlaceholder',
            )}
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
              onPlayTrack={playAlbumFrom}
              onPlayAll={playAlbum}
              onBack={goBack}
            />
          )}

          {!loading && !current && results && (
            <Box mt={2}>
              {results.artist?.length > 0 && (
                <Section title={translate('resources.tidal.sections.artists')}>
                  {results.artist.map((a) => (
                    <ArtistRow key={a.id} artist={a} onSelect={openArtist} />
                  ))}
                </Section>
              )}
              {results.album?.length > 0 && (
                <Section title={translate('resources.tidal.sections.albums')}>
                  {results.album.map((al) => (
                    <AlbumRow
                      key={al.id}
                      album={al}
                      onSelect={openAlbum}
                      onPlay={playAlbum}
                    />
                  ))}
                </Section>
              )}
              {results.song?.length > 0 && (
                <Section title={translate('resources.tidal.sections.songs')}>
                  {results.song.map((s) => (
                    <TrackRow key={s.id} track={s} onPlay={playTrack} />
                  ))}
                </Section>
              )}
              {!results.artist?.length &&
                !results.album?.length &&
                !results.song?.length && (
                  <Typography variant="body2" color="textSecondary">
                    {translate('ra.navigation.no_results')}
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
