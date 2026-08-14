import { useEffect, useState } from 'react'
import { useNotify, useTranslate } from 'react-admin'
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  MenuItem,
  Select,
  TextField,
  FormControl,
  InputLabel,
} from '@material-ui/core'
import { httpClient } from '../dataProvider'
import { useLibraries } from './useLibraries'

const TOOLS = ['yt-dlp', 'scdl', 'spotdl', 'bandcamp-dl']

const SubmitForm = ({ onSubmitted }) => {
  const translate = useTranslate()
  const notify = useNotify()
  const libraries = useLibraries()
  const [tool, setTool] = useState(TOOLS[0])
  const [sourceUrl, setSourceUrl] = useState('')
  const [libraryId, setLibraryId] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!libraryId && libraries.length > 0) {
      setLibraryId(libraries[0].id)
    }
  }, [libraries, libraryId])

  const handleSubmit = (e) => {
    e.preventDefault()
    setSubmitting(true)
    httpClient('/api/download', {
      method: 'POST',
      body: JSON.stringify({ tool, sourceUrl, libraryId: Number(libraryId) }),
    })
      .then(() => {
        notify('resources.downloader.messages.submitSuccess', {
          type: 'success',
        })
        setSourceUrl('')
        onSubmitted && onSubmitted()
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

  return (
    <Card>
      <CardHeader
        title={translate('resources.downloader.name', { _: 'Downloader' })}
      />
      <CardContent>
        <form onSubmit={handleSubmit}>
          <FormControl fullWidth margin="normal">
            <InputLabel id="downloader-tool-label">
              {translate('resources.downloader.fields.tool', { _: 'Tool' })}
            </InputLabel>
            <Select
              labelId="downloader-tool-label"
              value={tool}
              onChange={(e) => setTool(e.target.value)}
            >
              {TOOLS.map((t) => (
                <MenuItem key={t} value={t}>
                  {t}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            fullWidth
            margin="normal"
            required
            label={translate('resources.downloader.fields.sourceUrl', {
              _: 'URL',
            })}
            placeholder="https://…"
            value={sourceUrl}
            onChange={(e) => setSourceUrl(e.target.value)}
          />
          <FormControl fullWidth margin="normal">
            <InputLabel id="downloader-library-label">
              {translate('resources.downloader.fields.library', {
                _: 'Library',
              })}
            </InputLabel>
            <Select
              labelId="downloader-library-label"
              value={libraryId}
              onChange={(e) => setLibraryId(e.target.value)}
            >
              {libraries.map((lib) => (
                <MenuItem key={lib.id} value={lib.id}>
                  {lib.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button
            type="submit"
            color="primary"
            variant="contained"
            disabled={submitting || !sourceUrl || !libraryId}
            style={{ marginTop: 16 }}
          >
            {translate('resources.downloader.actions.submit', {
              _: 'Download',
            })}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

export default SubmitForm
