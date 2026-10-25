import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route } from 'react-router-dom'
import { ThemeProvider, createTheme } from '@material-ui/core/styles'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { httpClient } from '../dataProvider'
import TidalPage from './TidalPage'

const dispatch = vi.fn()
const notify = vi.fn()

vi.mock('react-redux', () => ({ useDispatch: () => dispatch }))
vi.mock('react-admin', () => ({
  Title: () => null,
  useNotify: () => notify,
  // Echo the key (plus smart_count) so assertions don't depend on the English bundle.
  useTranslate: () => (key, opts) =>
    opts?.smart_count != null ? `${key}:${opts.smart_count}` : key,
}))
vi.mock('../dataProvider', () => ({ httpClient: vi.fn() }))
vi.mock('./DownloadButton', () => ({ default: () => null }))

const artist = {
  id: 'ar1',
  name: 'Polo G',
  coverArt: 'ar-ar1',
  album: [
    { id: 'al1', name: 'Hall of Fame', year: 2021, songCount: 20 },
    { id: 'al2', name: 'Lay It Down', year: 2026, songCount: 1 },
    { id: 'al3', name: 'Die A Legend', year: 2019, songCount: 14 },
  ],
}

const album = {
  id: 'al1',
  name: 'Hall of Fame',
  artist: 'Polo G',
  artistId: 'ar1',
  year: 2021,
  song: [
    { id: '300', title: 'First', track: 1, duration: 200 },
    { id: '100', title: 'Second', track: 2, duration: 180 },
  ],
}

// withWidth (used by the grids, as in the native album grid) renders nothing until it can
// measure the viewport, which jsdom never lets it do.
const theme = createTheme({ props: { MuiWithWidth: { initialWidth: 'lg' } } })

const renderAt = (search) => {
  let location
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={[`/tidal${search}`]}>
        <TidalPage />
        <Route
          path="*"
          render={({ location: l }) => {
            location = l
            return null
          }}
        />
      </MemoryRouter>
    </ThemeProvider>,
  )
  return () => location
}

describe('TidalPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    httpClient.mockImplementation((url) => {
      if (url.startsWith('/api/tidal/artist/')) {
        return Promise.resolve({ json: artist })
      }
      if (url.startsWith('/api/tidal/album/')) {
        return Promise.resolve({ json: album })
      }
      return Promise.resolve({
        json: { artist: [artist], album: artist.album, song: album.song },
      })
    })
  })

  it('shows an artist with full albums and singles in separate grids, newest first', async () => {
    renderAt('?artist=ar1')

    expect(await screen.findByText('Polo G')).toBeInTheDocument()
    const albumsTitle = screen.getByText('resources.tidal.sections.albums')
    const singlesTitle = screen.getByText('resources.tidal.sections.singles')
    expect(albumsTitle).toBeInTheDocument()
    expect(singlesTitle).toBeInTheDocument()

    const names = screen
      .getAllByText(/Hall of Fame|Die A Legend|Lay It Down/)
      .map((el) => el.textContent)
    expect(names).toEqual(['Hall of Fame', 'Die A Legend', 'Lay It Down'])
  })

  it('opens an album from the artist grid through the URL', async () => {
    const location = renderAt('?q=polo&artist=ar1')

    fireEvent.click(await screen.findByText('Hall of Fame'))

    await waitFor(() => expect(location().search).toBe('?q=polo&album=al1'))
    expect(await screen.findByText('First')).toBeInTheDocument()
  })

  it('plays the album from the clicked track, in track order', async () => {
    renderAt('?album=al1')

    fireEvent.click(await screen.findByText('Second'))

    expect(dispatch).toHaveBeenCalledTimes(1)
    const action = dispatch.mock.calls[0][0]
    expect(action.id).toBe('tidal:100')
    expect(Object.keys(action.data)).toEqual(['tidal:300', 'tidal:100'])
  })

  it('renders search results as artist and album grids plus a song table', async () => {
    renderAt('?q=polo')

    expect(
      await screen.findByText('resources.tidal.sections.artists'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('resources.tidal.sections.albums'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('resources.tidal.sections.songs'),
    ).toBeInTheDocument()
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(httpClient).toHaveBeenCalledWith('/api/tidal/search?q=polo')
  })
})
