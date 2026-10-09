import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import TorrentCard from 'components/TorrentCard'
import CircularProgress from '@material-ui/core/CircularProgress'
import IconButton from '@material-ui/core/IconButton'
import Tooltip from '@material-ui/core/Tooltip'
import Button from '@material-ui/core/Button'
import Dialog from '@material-ui/core/Dialog'
import DialogTitle from '@material-ui/core/DialogTitle'
import DialogContent from '@material-ui/core/DialogContent'
import DialogActions from '@material-ui/core/DialogActions'
import TextField from '@material-ui/core/TextField'
import { TorrentListWrapper, CenteredGrid } from 'components/App/style'

import NoServerConnection from './NoServerConnection'
import AddFirstTorrent from './AddFirstTorrent'

function FilterIcon(props) {
  return (
    <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor" {...props}>
      <path d="M10 18h4v-2h-4v2zM3 6v2h18V6H3zm3 7h12v-2H6v2z" />
    </svg>
  )
}

export default function TorrentList({ isOffline, isLoading, sortABC, torrents, sortCategory }) {
  const { t } = useTranslation()
  const [filterQuery, setFilterQuery] = useState('')
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [tempQuery, setTempQuery] = useState('')

  const sortedTorrents = useMemo(() => {
    if (!torrents) return []
    const filtered = torrents.filter(torrent => {
      const matchesCategory = sortCategory === 'all' || torrent.category === sortCategory
      const matchesFilter = !filterQuery || (torrent.title || '').toLowerCase().includes(filterQuery.toLowerCase())
      return matchesCategory && matchesFilter
    })

    if (sortABC) {
      return [...filtered].sort((a, b) => (a.title || '').localeCompare(b.title || '') || a.hash.localeCompare(b.hash))
    }

    // Default: keep API order but stabilize by hash to prevent jumping
    return [...filtered].sort((a, b) => {
      const tsA = a.timestamp || 0
      const tsB = b.timestamp || 0
      if (tsA !== tsB) return tsB - tsA
      return a.hash.localeCompare(b.hash)
    })
  }, [torrents, sortCategory, sortABC, filterQuery])

  if (isLoading || isOffline || !torrents?.length) {
    return (
      <CenteredGrid>
        {isOffline ? (
          <NoServerConnection />
        ) : isLoading ? (
          <CircularProgress color='secondary' />
        ) : (
          !torrents.length && <AddFirstTorrent />
        )}
      </CenteredGrid>
    )
  }

  const handleOpenDialog = () => {
    setTempQuery(filterQuery)
    setIsDialogOpen(true)
  }

  const handleCloseDialog = () => {
    setIsDialogOpen(false)
  }

  const handleApplyFilter = () => {
    setFilterQuery(tempQuery)
    setIsDialogOpen(false)
  }

  const handleClearFilter = () => {
    setTempQuery('')
    setFilterQuery('')
    setIsDialogOpen(false)
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleApplyFilter()
    }
  }

  return (
    <TorrentListWrapper>
      <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', marginBottom: '10px', gap: '8px' }}>
        <Tooltip title={t('FilterByName') || 'Фильтр по имени'}>
          <IconButton
            color={filterQuery ? 'secondary' : 'default'}
            onClick={handleOpenDialog}
          >
            <FilterIcon />
          </IconButton>
        </Tooltip>
        {filterQuery && (
          <Button
            size="small"
            variant="outlined"
            onClick={handleClearFilter}
          >
            {t('Clear') || 'Очистить'}: {filterQuery} ✕
          </Button>
        )}
      </div>

      <Dialog open={isDialogOpen} onClose={handleCloseDialog} fullWidth maxWidth="xs">
        <DialogTitle>{t('FilterTitle') || 'Фильтрация по имени'}</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label={t('EnterTorrentName') || 'Введите часть названия торрента'}
            type="text"
            fullWidth
            value={tempQuery}
            onChange={(e) => setTempQuery(e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClearFilter} color="default">
            {t('Clear') || 'Очистить'}
          </Button>
          <Button onClick={handleCloseDialog} color="default">
            {t('Cancel') || 'Отмена'}
          </Button>
          <Button onClick={handleApplyFilter} color="primary" variant="contained">
            {t('OK') || 'OK'}
          </Button>
        </DialogActions>
      </Dialog>

      {sortedTorrents.map(torrent => (
        <TorrentCard key={torrent.hash} torrent={torrent} />
      ))}
    </TorrentListWrapper>
  )
}
