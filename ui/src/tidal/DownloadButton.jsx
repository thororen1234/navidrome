import { useState } from 'react'
import { useNotify, usePermissions, useTranslate } from 'react-admin'
import { IconButton, Menu, MenuItem, Tooltip, CircularProgress } from '@material-ui/core'
import CloudDownloadIcon from '@material-ui/icons/CloudDownload'
import { httpClient } from '../dataProvider'
import { useLibraries } from '../downloader/useLibraries'

// DownloadButton enqueues a "download to server" job for a Tidal track/album into the same
// queue the Downloader tab shows. Only rendered for admins, since it writes to the filesystem.
const DownloadButton = ({ tidalId, tidalKind }) => {
  const { permissions } = usePermissions()
  const translate = useTranslate()
  const notify = useNotify()
  const libraries = useLibraries()
  const [anchorEl, setAnchorEl] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  if (permissions !== 'admin') {
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
          error.body?.error ||
            error.message ||
            'resources.downloader.messages.submitError',
          { type: 'warning' },
        )
      })
      .finally(() => setSubmitting(false))
  }

  const handleClick = (e) => {
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
      <Tooltip
        title={translate('resources.downloader.actions.submit', {
          _: 'Download to server',
        })}
      >
        <IconButton size="small" onClick={handleClick} disabled={!libraries.length}>
          <CloudDownloadIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={() => setAnchorEl(null)}
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
