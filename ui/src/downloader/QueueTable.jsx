import { useCallback, useEffect, useImperativeHandle, useState } from 'react'
import { useSelector } from 'react-redux'
import { useNotify, useTranslate } from 'react-admin'
import {
  Button,
  Card,
  CardHeader,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Chip,
  LinearProgress,
  IconButton,
  Tooltip,
  Typography,
  Box,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import CancelIcon from '@material-ui/icons/Cancel'
import ReplayIcon from '@material-ui/icons/Replay'
import { httpClient } from '../dataProvider'
import { errorMessage } from './errorMessage'

// Each status gets an explicit palette color with its contrast text, rather than MUI's
// primary/secondary chip colors: themes are free to make "secondary" white (several do), which
// left error chips as blank white pills.
const statusChip = (theme, color) => ({
  backgroundColor: color,
  color: theme.palette.getContrastText(color),
})

const useStyles = makeStyles(
  (theme) => ({
    queued: {},
    canceled: {},
    downloading: statusChip(theme, theme.palette.info.main),
    completed: statusChip(theme, theme.palette.success.main),
    error: statusChip(theme, theme.palette.error.main),
  }),
  { name: 'NDDownloaderQueue' },
)

const ACTIVE = ['queued', 'downloading']
const RETRYABLE = ['error', 'canceled']

// Live-merges the fetched list with status pushed over SSE since that fetch, keyed by job id, so
// the table updates in place without a re-fetch on every progress tick.
const mergeJobs = (list, liveJobs) =>
  list.map((job) => (liveJobs[job.id] ? { ...job, ...liveJobs[job.id] } : job))

const formatTime = (iso) => {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

const QueueTable = ({ tableRef }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const notify = useNotify()
  const allLiveJobs = useSelector((state) => state.downloader?.jobs || {})
  const [{ jobs, loadedAt }, setList] = useState({ jobs: [], loadedAt: 0 })

  const load = useCallback(() => {
    const startedAt = Date.now()
    httpClient('/api/download')
      .then(({ json }) => setList({ jobs: json || [], loadedAt: startedAt }))
      .catch(() => {
        // Transient fetch failures are surfaced via the empty/last-known table state.
      })
  }, [])

  const liveJobs = Object.fromEntries(
    Object.entries(allLiveJobs).filter(([, e]) => e.receivedAt >= loadedAt),
  )

  useEffect(() => {
    load()
  }, [load])

  // Jobs submitted elsewhere (the Tidal page, another tab or admin) first show up as SSE events
  // for ids this table hasn't fetched; reload so they appear with their full details. The same
  // goes for a job reaching a terminal state, whose target path only the list has.
  const needsReload = Object.values(liveJobs).some((live) => {
    const job = jobs.find((j) => j.id === live.id)
    if (!job) return ACTIVE.includes(live.status)
    return !ACTIVE.includes(live.status) && job.status !== live.status
  })
  useEffect(() => {
    if (needsReload) load()
  }, [needsReload, load])

  useImperativeHandle(tableRef, () => ({ reload: load }), [load])

  const act = (request, successMsg, errorMsg) =>
    request
      .then(() => {
        notify(successMsg, { type: 'success' })
        load()
      })
      .catch((error) =>
        notify(errorMessage(error, errorMsg), { type: 'warning' }),
      )

  const handleCancel = (id) =>
    act(
      httpClient(`/api/download/${id}`, { method: 'DELETE' }),
      'resources.downloader.messages.cancelSuccess',
      'resources.downloader.messages.cancelError',
    )

  const handleRetry = (id) =>
    act(
      httpClient(`/api/download/${id}/retry`, { method: 'POST' }),
      'resources.downloader.messages.retrySuccess',
      'resources.downloader.messages.retryError',
    )

  const handleClearFinished = () =>
    act(
      httpClient('/api/download', { method: 'DELETE' }),
      'resources.downloader.messages.clearSuccess',
      'resources.downloader.messages.clearError',
    )

  const rows = mergeJobs(jobs, liveJobs)
  const hasFinished = rows.some((job) => !ACTIVE.includes(job.status))

  return (
    <Card>
      <CardHeader
        title={translate('resources.downloader.queue.title')}
        action={
          hasFinished && (
            <Button size="small" onClick={handleClearFinished}>
              {translate('resources.downloader.actions.clearFinished')}
            </Button>
          )
        }
      />
      {rows.length === 0 ? (
        <Box p={2}>
          <Typography variant="body2" color="textSecondary">
            {translate('resources.downloader.messages.empty')}
          </Typography>
        </Box>
      ) : (
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>
                {translate('resources.downloader.fields.tool')}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.sourceUrl')}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.createdAt')}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.status')}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.progress')}
              </TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((job) => {
              const detail =
                job.status === 'error'
                  ? job.error
                  : job.status === 'completed'
                    ? job.targetPath
                    : job.statusMessage
              return (
                <TableRow key={job.id}>
                  <TableCell>{job.tool}</TableCell>
                  <TableCell
                    style={{
                      maxWidth: 360,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    <Tooltip title={job.sourceUrl || ''}>
                      <span>
                        {job.sourceUrl || `${job.tidalKind}:${job.tidalId}`}
                      </span>
                    </Tooltip>
                    {detail && (
                      <Tooltip title={detail}>
                        <Typography
                          variant="caption"
                          display="block"
                          color={
                            job.status === 'error' ? 'error' : 'textSecondary'
                          }
                          noWrap
                        >
                          {detail}
                        </Typography>
                      </Tooltip>
                    )}
                  </TableCell>
                  <TableCell style={{ whiteSpace: 'nowrap' }}>
                    {formatTime(job.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={translate(
                        `resources.downloader.status.${job.status}`,
                        { _: job.status },
                      )}
                      className={classes[job.status]}
                    />
                  </TableCell>
                  <TableCell style={{ minWidth: 120 }}>
                    {job.status === 'downloading' &&
                      (job.progress > 0 ? (
                        <LinearProgress
                          variant="determinate"
                          value={Math.round(job.progress * 100)}
                        />
                      ) : (
                        <LinearProgress />
                      ))}
                  </TableCell>
                  <TableCell align="right" style={{ whiteSpace: 'nowrap' }}>
                    {ACTIVE.includes(job.status) && (
                      <Tooltip
                        title={translate('resources.downloader.actions.cancel')}
                      >
                        <IconButton
                          size="small"
                          onClick={() => handleCancel(job.id)}
                        >
                          <CancelIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                    {RETRYABLE.includes(job.status) && (
                      <Tooltip
                        title={translate('resources.downloader.actions.retry')}
                      >
                        <IconButton
                          size="small"
                          onClick={() => handleRetry(job.id)}
                        >
                          <ReplayIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
    </Card>
  )
}

export default QueueTable
