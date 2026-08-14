import { useState } from 'react'
import { Title, useNotify, useTranslate } from 'react-admin'
import {
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
  ListItemText,
  ListItemSecondaryAction,
  Divider,
  CircularProgress,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import { httpClient } from '../dataProvider'
import { usePlayer } from './usePlayer'
import DownloadButton from './DownloadButton'

const msToTime = (seconds) => {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

const TrackRow = ({ track, onPlay }) => (
  <ListItem divider>
    <ListItemText
      primary={track.title}
      secondary={`${track.artist || ''}${track.album ? ' - ' + track.album : ''}${track.duration ? ' · ' + msToTime(track.duration) : ''
        }`}
    />
    <ListItemSecondaryAction>
      <IconButton size="small" onClick={() => onPlay(track.id)}>
        <PlayArrowIcon fontSize="small" />
      </IconButton>
      <DownloadButton tidalId={track.id} tidalKind="track" />
    </ListItemSecondaryAction>
  </ListItem>
)

const AlbumRow = ({ album, onSelect }) => (
  <ListItem button divider onClick={() => onSelect(album)}>
    <ListItemText
      primary={album.name}
      secondary={`${album.artist || ''}${album.songCount ? ' · ' + album.songCount + ' tracks' : ''
        }`}
    />
    <ListItemSecondaryAction>
      <DownloadButton tidalId={album.id} tidalKind="album" />
    </ListItemSecondaryAction>
  </ListItem>
)

const ArtistRow = ({ artist }) => (
  <ListItem divider>
    <ListItemText primary={artist.name} />
  </ListItem>
)

const AlbumDetail = ({ album, onPlay, onBack }) => {
  const translate = useTranslate()
  return (
    <Box mt={2}>
      <Typography variant="h6">{album.name}</Typography>
      <Typography variant="body2" color="textSecondary" gutterBottom>
        {album.artist}
      </Typography>
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

const TidalPage = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const player = usePlayer()
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(null)
  const [selectedAlbum, setSelectedAlbum] = useState(null)

  const runSearch = () => {
    if (!query.trim()) return
    setLoading(true)
    setSelectedAlbum(null)
    httpClient(`/api/tidal/search?q=${encodeURIComponent(query)}`)
      .then(({ json }) => setResults(json))
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
        setResults(null)
      })
      .finally(() => setLoading(false))
  }

  const openAlbum = (album) => {
    httpClient(`/api/tidal/album/${album.id}`)
      .then(({ json }) => setSelectedAlbum(json))
      .catch((error) => {
        notify(error.message || 'ra.page.error', { type: 'warning' })
      })
  }

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

          {!loading && selectedAlbum && (
            <AlbumDetail
              album={selectedAlbum}
              onPlay={player.play}
              onBack={() => setSelectedAlbum(null)}
            />
          )}

          {!loading && !selectedAlbum && results && (
            <Box mt={2}>
              {results.artist?.length > 0 && (
                <>
                  <Typography variant="subtitle1">Artists</Typography>
                  <List dense>
                    {results.artist.map((a) => (
                      <ArtistRow key={a.id} artist={a} />
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
                      <AlbumRow key={al.id} album={al} onSelect={openAlbum} />
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
                      <TrackRow key={s.id} track={s} onPlay={player.play} />
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

      {player.src && (
        // eslint-disable-next-line jsx-a11y/media-has-caption
        <audio
          src={player.src}
          controls
          autoPlay
          onEnded={player.stop}
          style={{ width: '100%', marginTop: 16 }}
        />
      )}
    </Box>
  )
}

export default TidalPage
