import { useCallback, useEffect, useImperativeHandle, useState } from 'react'
import { useSelector } from 'react-redux'
import { useNotify, useTranslate } from 'react-admin'
import {
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
import CancelIcon from '@material-ui/icons/Cancel'
import { httpClient } from '../dataProvider'

const STATUS_COLORS = {
  queued: 'default',
  downloading: 'primary',
  completed: 'primary',
  error: 'secondary',
  canceled: 'default',
}

// Live-merges the fetched list with any newer status pushed over SSE, keyed by job id, so the
// table updates in place without a re-fetch on every progress tick.
const mergeJobs = (list, liveJobs) =>
  list.map((job) => ({ ...job, ...liveJobs[job.id] }))

const QueueTable = ({ tableRef }) => {
  const translate = useTranslate()
  const notify = useNotify()
  const liveJobs = useSelector((state) => state.downloader?.jobs || {})
  const [jobs, setJobs] = useState([])

  const load = useCallback(() => {
    httpClient('/api/download')
      .then(({ json }) => setJobs(json || []))
      .catch(() => {
        // Transient fetch failures are surfaced via the empty/last-known table state.
      })
  }, [])

  useEffect(() => {
    load()
  }, [load])

  useImperativeHandle(tableRef, () => ({ reload: load }), [load])

  const handleCancel = (id) => {
    httpClient(`/api/download/${id}`, { method: 'DELETE' })
      .then(() => {
        notify('resources.downloader.messages.cancelSuccess', {
          type: 'success',
        })
        load()
      })
      .catch((error) => {
        notify(
          error.body?.error ||
            error.message ||
            'resources.downloader.messages.cancelError',
          { type: 'warning' },
        )
      })
  }

  const rows = mergeJobs(jobs, liveJobs)

  return (
    <Card>
      <CardHeader title={translate('menu.albumList', { _: 'Queue' })} />
      {rows.length === 0 ? (
        <Box p={2}>
          <Typography variant="body2" color="textSecondary">
            {translate('resources.downloader.messages.empty', {
              _: 'No downloads yet',
            })}
          </Typography>
        </Box>
      ) : (
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>
                {translate('resources.downloader.fields.tool', {
                  _: 'Tool',
                })}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.sourceUrl', {
                  _: 'Source',
                })}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.status', {
                  _: 'Status',
                })}
              </TableCell>
              <TableCell>
                {translate('resources.downloader.fields.progress', {
                  _: 'Progress',
                })}
              </TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((job) => (
              <TableRow key={job.id}>
                <TableCell>{job.tool}</TableCell>
                <TableCell
                  style={{
                    maxWidth: 320,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {job.sourceUrl || `${job.tidalKind}:${job.tidalId}`}
                </TableCell>
                <TableCell>
                  <Tooltip title={job.statusMessage || job.error || ''}>
                    <Chip
                      size="small"
                      label={translate(
                        `resources.downloader.status.${job.status}`,
                        { _: job.status },
                      )}
                      color={STATUS_COLORS[job.status] || 'default'}
                    />
                  </Tooltip>
                </TableCell>
                <TableCell style={{ minWidth: 120 }}>
                  {job.status === 'downloading' ? (
                    <LinearProgress
                      variant="determinate"
                      value={Math.round((job.progress || 0) * 100)}
                    />
                  ) : null}
                </TableCell>
                <TableCell align="right">
                  {job.status === 'queued' && (
                    <Tooltip
                      title={translate('resources.downloader.actions.cancel', {
                        _: 'Cancel',
                      })}
                    >
                      <IconButton
                        size="small"
                        onClick={() => handleCancel(job.id)}
                      >
                        <CancelIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Card>
  )
}

export default QueueTable
