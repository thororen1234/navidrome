import { useState } from 'react'
import { useNotify, usePermissions, useTranslate } from 'react-admin'
import {
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
  CircularProgress,
} from '@material-ui/core'
import CloudDownloadIcon from '@material-ui/icons/CloudDownload'
import { httpClient } from '../dataProvider'
import { errorMessage } from '../downloader/errorMessage'
import config from '../config'
import { useLibraries } from '../downloader/useLibraries'

// DownloadButton enqueues a "download to server" job for a Tidal track/album into the same
// queue the Downloader tab shows. Only rendered for admins, since it writes to the filesystem,
// and only when the server accepts these jobs (Tidal.AllowDownload with the Downloader enabled).
const DownloadButton = ({ tidalId, tidalKind }) => {
  const { permissions } = usePermissions()
  const translate = useTranslate()
  const notify = useNotify()
  const allowed = permissions === 'admin' && config.tidalAllowDownload
  const libraries = useLibraries(allowed)
  const [anchorEl, setAnchorEl] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  if (!allowed) {
    return null
  }

  const submit = (libraryId) => {
    setAnchorEl(null)
    setSubmitting(true)
    httpClient('/api/tidal/download', {
      method: 'POST',
      body: JSON.stringify({ tidalId, tidalKind, libraryId }),
    })
      .then(() => {
        notify('resources.downloader.messages.submitSuccess', {
          type: 'success',
        })
      })
      .catch((error) => {
        notify(
          errorMessage(error, 'resources.downloader.messages.submitError'),
          { type: 'warning' },
        )
      })
      .finally(() => setSubmitting(false))
  }

  const handleClick = (e) => {
    e.stopPropagation() // rows are clickable (open album/artist)
    if (libraries.length <= 1) {
      submit(libraries[0]?.id)
      return
    }
    setAnchorEl(e.currentTarget)
  }

  if (submitting) {
    return <CircularProgress size={20} />
  }

  return (
    <>
      <Tooltip title={translate('resources.tidal.actions.downloadToServer')}>
        <IconButton
          size="small"
          onClick={handleClick}
          disabled={!libraries.length}
        >
          <CloudDownloadIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={() => setAnchorEl(null)}
        onClick={(e) => e.stopPropagation()}
      >
        {libraries.map((lib) => (
          <MenuItem key={lib.id} onClick={() => submit(lib.id)}>
            {lib.name}
          </MenuItem>
        ))}
      </Menu>
    </>
  )
}

export default DownloadButton
