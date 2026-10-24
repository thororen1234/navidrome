import { useEffect, useState } from 'react'
import { useDataProvider } from 'react-admin'

// One shared fetch for every caller: the Tidal page renders a DownloadButton per result row, and
// each used to fire its own library request. Libraries change rarely and there's no SSE signal
// for them, so the result is kept for the page's lifetime (a failed fetch is retried next time).
let librariesPromise = null

const fetchLibraries = (dataProvider) => {
  if (!librariesPromise) {
    librariesPromise = dataProvider
      .getList('library', {
        pagination: { page: 1, perPage: 100 },
        sort: { field: 'name', order: 'ASC' },
        filter: {},
      })
      .then(({ data }) => data)
      .catch((e) => {
        librariesPromise = null
        throw e
      })
  }
  return librariesPromise
}

// useLibraries returns the libraries an admin can pick as a download target. Pass
// enabled=false to skip the fetch entirely (the library endpoint is admin-only).
export const useLibraries = (enabled = true) => {
  const dataProvider = useDataProvider()
  const [libraries, setLibraries] = useState([])

  useEffect(() => {
    if (!enabled) return undefined
    let active = true
    fetchLibraries(dataProvider)
      .then((data) => {
        if (active) setLibraries(data)
      })
      .catch(() => {
        if (active) setLibraries([])
      })
    return () => {
      active = false
    }
  }, [dataProvider, enabled])

  return libraries
}
