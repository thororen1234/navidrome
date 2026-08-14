import { useCallback, useEffect, useState } from 'react'
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
  Button,
  Chip,
  CircularProgress,
} from '@material-ui/core'
import { httpClient } from '../dataProvider'

const ACTIONS = ['install', 'upgrade', 'repair']

const ToolStatusPanel = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const liveTools = useSelector((state) => state.downloader?.tools || {})
  const [tools, setTools] = useState([])
  const [runningTool, setRunningTool] = useState(null)

  const load = useCallback(() => {
    httpClient('/api/download/tools')
      .then(({ json }) => setTools(json || []))
      .catch(() => {
        // Transient fetch failures are surfaced via the last-known table state.
      })
  }, [])

  // Reloads on mount, and again whenever an install/upgrade/repair broadcast arrives (including
  // ones started from another tab or user), so a just-finished action's new version shows up.
  useEffect(() => {
    load()
  }, [load, liveTools])

  const handleAction = (tool, action) => {
    setRunningTool(tool)
    httpClient(`/api/download/tools/${tool}/${action}`, { method: 'POST' })
      .then(() => {
        notify('resources.downloader.messages.toolActionSuccess', {
          type: 'success',
          messageArgs: { tool, action },
        })
        load()
      })
      .catch((error) => {
        notify(
          error.body?.error ||
            error.message ||
            'resources.downloader.messages.toolActionError',
          { type: 'warning', messageArgs: { tool, action } },
        )
      })
      .finally(() => setRunningTool(null))
  }

  return (
    <Card>
      <CardHeader
        title={translate('resources.downloader.tools.title', {
          _: 'Downloader Tools',
        })}
      />
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>
              {translate('resources.downloader.fields.tool', { _: 'Tool' })}
            </TableCell>
            <TableCell>
              {translate('resources.downloader.fields.status', {
                _: 'Status',
              })}
            </TableCell>
            <TableCell align="right" />
          </TableRow>
        </TableHead>
        <TableBody>
          {tools.map((t) => {
            const live = liveTools[t.tool]
            const busy = runningTool === t.tool || live?.running
            return (
              <TableRow key={t.tool}>
                <TableCell>{t.tool}</TableCell>
                <TableCell>
                  {t.installed ? (
                    <Chip size="small" color="primary" label={t.version || 'OK'} />
                  ) : (
                    <Chip
                      size="small"
                      label={translate(
                        'resources.downloader.messages.notInstalled',
                        { _: 'Not installed' },
                      )}
                    />
                  )}
                </TableCell>
                <TableCell align="right">
                  {busy ? (
                    <CircularProgress size={20} />
                  ) : (
                    ACTIONS.map((action) => (
                      <Button
                        key={action}
                        size="small"
                        onClick={() => handleAction(t.tool, action)}
                      >
                        {translate(`resources.downloader.actions.${action}`, {
                          _: action,
                        })}
                      </Button>
                    ))
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </Card>
  )
}

export default ToolStatusPanel
