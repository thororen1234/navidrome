import { useState } from 'react'
import { useDispatch } from 'react-redux'
import { useTranslate } from 'react-admin'
import {
  Button,
  Card,
  CardContent,
  GridList,
  GridListTile,
  GridListTileBar,
  IconButton,
  Link,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
  useMediaQuery,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import withWidth from '@material-ui/core/withWidth'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import ShuffleIcon from '@material-ui/icons/Shuffle'
import RedoIcon from '@material-ui/icons/Redo'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import AlbumIcon from '@material-ui/icons/Album'
import PersonIcon from '@material-ui/icons/Person'
import DownloadButton from './DownloadButton'
import { tidalArtistImageUrl, tidalCoverArtUrl } from './urls'
import {
  addTrackListToQueue,
  playTrackList,
  playTrackListNext,
  shuffleTrackList,
} from './playback'

// These views mirror the native album grid (album/AlbumGridView), artist header
// (artist/DesktopArtistDetails) and album header/song list (album/AlbumDetails, AlbumSongs).
// They can't reuse those components directly: those read library records through
// react-admin resources and the local artwork pipeline, while Tidal items only exist as
// TidalSubsonic search results.

const formatDuration = (seconds) => {
  if (!seconds) return ''
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.round(seconds % 60)
  const mm = h ? m.toString().padStart(2, '0') : m
  return `${h ? h + ':' : ''}${mm}:${s.toString().padStart(2, '0')}`
}

const getColsForWidth = (width) => {
  if (width === 'xs') return 2
  if (width === 'sm') return 3
  if (width === 'md') return 4
  if (width === 'lg') return 6
  return 8
}

const useStyles = makeStyles(
  (theme) => ({
    grid: { margin: theme.spacing(1, 0, 3) },
    sectionTitle: { margin: theme.spacing(3, 0, 1) },
    tile: { cursor: 'pointer' },
    coverBox: {
      position: 'relative',
      width: '100%',
      aspectRatio: '1',
      overflow: 'hidden',
      backgroundColor:
        theme.palette.type === 'dark'
          ? 'rgba(255,255,255,0.06)'
          : 'rgba(0,0,0,0.06)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      '&:hover $tileBar, &:focus-within $tileBar': {
        opacity: 1,
        pointerEvents: 'auto',
      },
    },
    round: { borderRadius: '50%' },
    coverImg: {
      position: 'absolute',
      top: 0,
      left: 0,
      width: '100%',
      height: '100%',
      objectFit: 'cover',
    },
    placeholderIcon: { fontSize: '3em', opacity: 0.4 },
    tileBar: {
      transition: 'all 150ms ease-out',
      opacity: 0,
      pointerEvents: 'none',
      background:
        'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
    },
    tileBarMobile: {
      background:
        'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
    },
    tileButton: { color: 'white' },
    tileTitle: {
      fontSize: '14px',
      color: theme.palette.type === 'dark' ? '#eee' : 'black',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
      marginTop: theme.spacing(0.5),
    },
    tileSubtitle: {
      fontSize: '12px',
      color: theme.palette.type === 'dark' ? '#c5c5c5' : '#696969',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    centered: { textAlign: 'center' },
    header: {
      display: 'flex',
      gap: theme.spacing(3),
      padding: theme.spacing(2),
      alignItems: 'center',
      [theme.breakpoints.down('xs')]: {
        flexDirection: 'column',
        alignItems: 'flex-start',
      },
    },
    artistImage: { width: '12rem', minWidth: '12rem' },
    albumImage: {
      width: '15em',
      minWidth: '15em',
      [theme.breakpoints.down('sm')]: { width: '10em', minWidth: '10em' },
    },
    headerMeta: {
      color: theme.palette.text.secondary,
      marginTop: theme.spacing(0.5),
    },
    actions: {
      display: 'flex',
      flexWrap: 'wrap',
      gap: theme.spacing(1),
      margin: theme.spacing(2, 0, 1),
    },
    trackRow: {
      cursor: 'pointer',
      '&:hover $trackNumber': { display: 'none' },
      '&:hover $trackPlay': { display: 'inline-flex' },
    },
    trackNumber: { color: theme.palette.text.secondary },
    trackPlay: { display: 'none', padding: 0 },
    numberCell: { width: '3em', textAlign: 'center' },
    durationCell: { width: '5em', whiteSpace: 'nowrap' },
    actionCell: { width: '1%', whiteSpace: 'nowrap' },
    secondaryText: { color: theme.palette.text.secondary },
  }),
  { name: 'NDTidalViews' },
)

// Cover shows an image with a placeholder icon behind it, which stays visible if the image is
// missing or fails to load.
const Cover = ({ src, round, placeholder: Placeholder, alt, children }) => {
  const classes = useStyles()
  const [failed, setFailed] = useState(false)
  return (
    <div className={`${classes.coverBox} ${round ? classes.round : ''}`}>
      <Placeholder className={classes.placeholderIcon} />
      {src && !failed && (
        <img
          src={src}
          alt={alt}
          loading="lazy"
          className={classes.coverImg}
          onError={() => setFailed(true)}
        />
      )}
      {children}
    </div>
  )
}

const TileBar = ({ children }) => {
  const classes = useStyles()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'), {
    noSsr: true,
  })
  return (
    <GridListTileBar
      className={isDesktop ? classes.tileBar : classes.tileBarMobile}
      onClick={(e) => e.stopPropagation()}
      subtitle={children}
    />
  )
}

const albumSubtitle = (album, showArtist, translate) =>
  [
    showArtist ? album.artist : album.year,
    album.songCount > 1 &&
      translate('resources.tidal.messages.trackCount', {
        smart_count: album.songCount,
      }),
  ]
    .filter(Boolean)
    .join(' · ')

const AlbumTile = ({ album, showArtist, onOpen, onPlay }) => {
  const classes = useStyles()
  const translate = useTranslate()
  return (
    <div className={classes.tile} onClick={() => onOpen(album)}>
      <Cover
        src={tidalCoverArtUrl(album.coverArt, 300)}
        placeholder={AlbumIcon}
        alt={album.name}
      >
        <TileBar>
          <IconButton
            size="small"
            className={classes.tileButton}
            onClick={() => onPlay(album)}
          >
            <PlayArrowIcon />
          </IconButton>
          <DownloadButton
            tidalId={album.id}
            tidalKind="album"
            className={classes.tileButton}
          />
        </TileBar>
      </Cover>
      <Typography className={classes.tileTitle} title={album.name}>
        {album.name}
      </Typography>
      <Typography className={classes.tileSubtitle}>
        {albumSubtitle(album, showArtist, translate)}
      </Typography>
    </div>
  )
}

export const AlbumGrid = withWidth()(({
  width,
  albums,
  showArtist,
  onOpen,
  onPlay,
}) => {
  const classes = useStyles()
  return (
    <GridList
      component="div"
      cellHeight="auto"
      cols={getColsForWidth(width)}
      spacing={20}
      className={classes.grid}
    >
      {albums.map((album) => (
        <GridListTile key={album.id}>
          <AlbumTile
            album={album}
            showArtist={showArtist}
            onOpen={onOpen}
            onPlay={onPlay}
          />
        </GridListTile>
      ))}
    </GridList>
  )
})

export const ArtistGrid = withWidth()(({ width, artists, onOpen }) => {
  const classes = useStyles()
  const translate = useTranslate()
  return (
    <GridList
      component="div"
      cellHeight="auto"
      cols={getColsForWidth(width)}
      spacing={20}
      className={classes.grid}
    >
      {artists.map((artist) => (
        <GridListTile key={artist.id}>
          <div
            className={`${classes.tile} ${classes.centered}`}
            onClick={() => onOpen(artist)}
          >
            <Cover
              round
              src={tidalArtistImageUrl(artist, 300)}
              placeholder={PersonIcon}
              alt={artist.name}
            />
            <Typography className={classes.tileTitle} title={artist.name}>
              {artist.name}
            </Typography>
            {artist.albumCount > 0 && (
              <Typography className={classes.tileSubtitle}>
                {translate('resources.tidal.messages.albumCount', {
                  smart_count: artist.albumCount,
                })}
              </Typography>
            )}
          </div>
        </GridListTile>
      ))}
    </GridList>
  )
})

export const SectionTitle = ({ children }) => {
  const classes = useStyles()
  return (
    <Typography variant="h6" className={classes.sectionTitle}>
      {children}
    </Typography>
  )
}

export const ArtistHeader = ({ artist, albums }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const image =
    tidalArtistImageUrl(artist, 600) ||
    tidalCoverArtUrl(albums.find((a) => a.coverArt)?.coverArt, 600)
  return (
    <Card>
      <div className={classes.header}>
        <div className={classes.artistImage}>
          <Cover round src={image} placeholder={PersonIcon} alt={artist.name} />
        </div>
        <CardContent>
          <Typography variant="h5" component="h5">
            {artist.name}
          </Typography>
          {albums.length > 0 && (
            <Typography variant="body2" className={classes.headerMeta}>
              {translate('resources.tidal.messages.albumCount', {
                smart_count: albums.length,
              })}
            </Typography>
          )}
        </CardContent>
      </div>
    </Card>
  )
}

export const AlbumHeader = ({ album, onOpenArtist }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const dispatch = useDispatch()
  const songs = album.song || []
  const totalDuration =
    album.duration || songs.reduce((sum, s) => sum + (s.duration || 0), 0)
  return (
    <Card>
      <div className={classes.header}>
        <div className={classes.albumImage}>
          <Cover
            src={tidalCoverArtUrl(album.coverArt, 600)}
            placeholder={AlbumIcon}
            alt={album.name}
          />
        </div>
        <CardContent>
          <Typography variant="h5" component="h5">
            {album.name}
          </Typography>
          <Typography variant="subtitle1">
            {album.artistId ? (
              <Link
                component="button"
                variant="subtitle1"
                color="inherit"
                onClick={() =>
                  onOpenArtist({ id: album.artistId, name: album.artist })
                }
              >
                {album.artist}
              </Link>
            ) : (
              album.artist
            )}
          </Typography>
          <Typography variant="body2" className={classes.headerMeta}>
            {[
              album.year,
              songs.length > 0 &&
                translate('resources.tidal.messages.trackCount', {
                  smart_count: songs.length,
                }),
              formatDuration(totalDuration),
            ]
              .filter(Boolean)
              .join(' · ')}
          </Typography>
          <div className={classes.actions}>
            <Button
              size="small"
              color="primary"
              variant="contained"
              startIcon={<PlayArrowIcon />}
              disabled={!songs.length}
              onClick={() => playTrackList(dispatch, songs)}
            >
              {translate('resources.album.actions.playAll')}
            </Button>
            <Button
              size="small"
              startIcon={<ShuffleIcon />}
              disabled={songs.length < 2}
              onClick={() => shuffleTrackList(dispatch, songs)}
            >
              {translate('resources.album.actions.shuffle')}
            </Button>
            <Button
              size="small"
              startIcon={<RedoIcon />}
              disabled={!songs.length}
              onClick={() => playTrackListNext(dispatch, songs)}
            >
              {translate('resources.album.actions.playNext')}
            </Button>
            <Button
              size="small"
              startIcon={<QueueMusicIcon />}
              disabled={!songs.length}
              onClick={() => addTrackListToQueue(dispatch, songs)}
            >
              {translate('resources.album.actions.addToQueue')}
            </Button>
            <DownloadButton tidalId={album.id} tidalKind="album" showLabel />
          </div>
        </CardContent>
      </div>
    </Card>
  )
}

// TrackTable lists songs like the native album song list: hovering a row swaps its number for
// a play icon, and clicking a row plays it. showAlbum/showArtist add columns for search
// results, where tracks come from different albums.
export const TrackTable = ({
  tracks,
  onPlay,
  showAlbum = false,
  showArtist = false,
}) => {
  const classes = useStyles()
  const translate = useTranslate()
  const isXs = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  return (
    <Card>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell className={classes.numberCell}>#</TableCell>
            <TableCell>{translate('resources.song.fields.title')}</TableCell>
            {showArtist && !isXs && (
              <TableCell>{translate('resources.song.fields.artist')}</TableCell>
            )}
            {showAlbum && !isXs && (
              <TableCell>{translate('resources.song.fields.album')}</TableCell>
            )}
            <TableCell className={classes.durationCell}>
              {translate('resources.song.fields.duration')}
            </TableCell>
            <TableCell className={classes.actionCell} />
          </TableRow>
        </TableHead>
        <TableBody>
          {tracks.map((track, idx) => (
            <TableRow
              key={track.id}
              hover
              className={classes.trackRow}
              onClick={() => onPlay(track)}
            >
              <TableCell className={classes.numberCell}>
                <span className={classes.trackNumber}>
                  {track.track || idx + 1}
                </span>
                <IconButton size="small" className={classes.trackPlay}>
                  <PlayArrowIcon fontSize="small" />
                </IconButton>
              </TableCell>
              <TableCell>
                {track.title}
                {(showArtist || showAlbum) && isXs && (
                  <Typography
                    variant="caption"
                    display="block"
                    className={classes.secondaryText}
                  >
                    {[showArtist && track.artist, showAlbum && track.album]
                      .filter(Boolean)
                      .join(' · ')}
                  </Typography>
                )}
              </TableCell>
              {showArtist && !isXs && (
                <TableCell className={classes.secondaryText}>
                  {track.artist}
                </TableCell>
              )}
              {showAlbum && !isXs && (
                <TableCell className={classes.secondaryText}>
                  {track.album}
                </TableCell>
              )}
              <TableCell
                className={`${classes.durationCell} ${classes.secondaryText}`}
              >
                {formatDuration(track.duration)}
              </TableCell>
              <TableCell
                className={classes.actionCell}
                onClick={(e) => e.stopPropagation()}
              >
                <DownloadButton tidalId={track.id} tidalKind="track" />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
