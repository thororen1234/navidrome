import { useEffect, useState } from 'react'
import { useDataProvider } from 'react-admin'

// useLibraries fetches the list of libraries an admin can pick as a download target. There's no
// SSE signal for library changes here, so it's a plain fetch-on-mount; libraries change rarely.
export const useLibraries = () => {
  const dataProvider = useDataProvider()
  const [libraries, setLibraries] = useState([])

  useEffect(() => {
    let active = true
    dataProvider
      .getList('library', {
        pagination: { page: 1, perPage: 100 },
        sort: { field: 'name', order: 'ASC' },
        filter: {},
      })
      .then(({ data }) => {
        if (active) setLibraries(data)
      })
      .catch(() => {
        if (active) setLibraries([])
      })
    return () => {
      active = false
    }
  }, [dataProvider])

  return libraries
}
