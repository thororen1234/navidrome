import { useRef } from 'react'
import { Title, useTranslate } from 'react-admin'
import { Grid, Box } from '@material-ui/core'
import SubmitForm from './SubmitForm'
import ToolStatusPanel from './ToolStatusPanel'
import QueueTable from './QueueTable'

const DownloaderPage = () => {
  const translate = useTranslate()
  const tableRef = useRef(null)

  return (
    <Box mt={2}>
      <Title
        title={
          'Navidrome - ' +
          translate('resources.downloader.name', { _: 'Downloader' })
        }
      />
      <Grid container spacing={3}>
        <Grid item xs={12} md={5}>
          <SubmitForm onSubmitted={() => tableRef.current?.reload()} />
        </Grid>
        <Grid item xs={12} md={7}>
          <ToolStatusPanel />
        </Grid>
        <Grid item xs={12}>
          <QueueTable tableRef={tableRef} />
        </Grid>
      </Grid>
    </Box>
  )
}

export default DownloaderPage
