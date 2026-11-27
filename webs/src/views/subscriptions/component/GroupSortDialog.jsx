import { useState, useEffect, useCallback, useMemo } from 'react';
import { useTheme } from '@mui/material/styles';
import useMediaQuery from '@mui/material/useMediaQuery';

// material-ui
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import Chip from '@mui/material/Chip';
import Alert from '@mui/material/Alert';
import Snackbar from '@mui/material/Snackbar';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import TextField from '@mui/material/TextField';
import InputAdornment from '@mui/material/InputAdornment';
import CircularProgress from '@mui/material/CircularProgress';
import Stack from '@mui/material/Stack';
import Paper from '@mui/material/Paper';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';

// icons
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import DragIndicatorIcon from '@mui/icons-material/DragIndicator';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import SaveIcon from '@mui/icons-material/Save';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import VerticalAlignTopIcon from '@mui/icons-material/VerticalAlignTop';
import VerticalAlignBottomIcon from '@mui/icons-material/VerticalAlignBottom';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';

// drag and drop
import { DragDropContext, Droppable, Draggable } from '@hello-pangea/dnd';

// project imports
import { getGroupSortGroups, getGroupSortDetail, saveGroupAirportSort, saveGroupNodeOrder } from 'api/groupSort';
import ConfirmDialog from './ConfirmDialog';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getReadableTextTokens, getSurfaceTokens } from 'themes/surfaceTokens';
import { withAlpha } from 'utils/colorUtils';
import { useTranslation } from 'react-i18next';

function areItemOrdersEqual(left, right, key) {
  if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) {
    return false;
  }
  for (let i = 0; i < left.length; i += 1) {
    if (left[i][key] !== right[i][key]) {
      return false;
    }
  }
  return true;
}

export default function GroupSortDialog({ open, onClose, showMessage }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isDark } = useResolvedColorScheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const { palette, dialogSurface, dialogSurfaceGradient, mutedPanelSurface, nestedPanelSurface, panelBorder } = getSurfaceTokens(
    theme,
    isDark
  );
  const { primaryText, secondaryText, tertiaryText } = getReadableTextTokens(theme, isDark);

  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [mode, setMode] = useState('nodes');
  const [detail, setDetail] = useState(null);
  const [drafts, setDrafts] = useState({});
  const [loadStatus, setLoadStatus] = useState('');
  const [detailVersion, setDetailVersion] = useState(0);
  const [saving, setSaving] = useState('');
  const [searchText, setSearchText] = useState('');
  const [itemSearchText, setItemSearchText] = useState('');
  const [pendingAction, setPendingAction] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  const handleSnackbar = useCallback(
    (message, severity) => {
      if (showMessage) {
        showMessage(message, severity);
      } else {
        setSnackbar({ open: true, message, severity });
      }
    },
    [showMessage]
  );

  const loadGroups = useCallback(async () => {
    try {
      const res = await getGroupSortGroups();
      setGroups(res.data || []);
    } catch (err) {
      console.error('Failed to load groups:', err);
      handleSnackbar(t('subscriptions.sort.messages.loadGroupsFailed'), 'error');
    }
  }, [handleSnackbar, t]);

  useEffect(() => {
    if (open) {
      loadGroups();
    }
  }, [open, loadGroups]);

  useEffect(() => {
    if (!open || !selectedGroup) return;
    let active = true;
    setLoadStatus('loading');
    getGroupSortDetail(selectedGroup)
      .then(({ data }) => {
        if (!active) return;
        setDetail(data);
        setLoadStatus('ready');
      })
      .catch(() => {
        if (!active) return;
        setLoadStatus('error');
      });
    return () => {
      active = false;
    };
  }, [open, selectedGroup, detailVersion]);

  const isNodeMode = mode === 'nodes';
  const currentDetail = detail?.groupName === selectedGroup ? detail : null;
  const items = useMemo(() => {
    if (!currentDetail) return [];
    return drafts[mode] ?? currentDetail[mode] ?? [];
  }, [currentDetail, drafts, mode]);

  const hasNodeChanges = useMemo(
    () => Boolean(currentDetail && drafts.nodes && !areItemOrdersEqual(drafts.nodes, currentDetail.nodes || [], 'id')),
    [currentDetail, drafts.nodes]
  );
  const hasAirportChanges = useMemo(
    () => Boolean(currentDetail && drafts.airports && !areItemOrdersEqual(drafts.airports, currentDetail.airports || [], 'airportId')),
    [currentDetail, drafts.airports]
  );
  const isCurrentModeDirty = isNodeMode ? hasNodeChanges : hasAirportChanges;
  const hasAnyUnsavedChanges = hasNodeChanges || hasAirportChanges;
  const hasSavedCurrentMode = isNodeMode ? Boolean(currentDetail?.hasNodeOrder) : Boolean(currentDetail?.hasAirportSort);

  const performClose = useCallback(() => {
    setSelectedGroup('');
    setDetail(null);
    setDrafts({});
    setLoadStatus('');
    setMode('nodes');
    setSearchText('');
    setItemSearchText('');
    setPendingAction(null);
    onClose();
  }, [onClose]);

  const performSelectGroup = useCallback((groupName) => {
    setLoadStatus('loading');
    setDrafts({});
    setItemSearchText('');
    setSelectedGroup(groupName);
  }, []);

  const handleClose = () => {
    if (saving) return;
    if (hasAnyUnsavedChanges) {
      setPendingAction({ type: 'close' });
      return;
    }
    performClose();
  };

  const handleGroupClick = (groupName) => {
    if (groupName === selectedGroup || saving) return;
    if (hasAnyUnsavedChanges) {
      setPendingAction({ type: 'selectGroup', groupName });
      return;
    }
    performSelectGroup(groupName);
  };

  const handleConfirmDiscard = () => {
    const action = pendingAction;
    setPendingAction(null);
    if (!action) return;
    if (action.type === 'close') {
      performClose();
    } else if (action.type === 'selectGroup') {
      performSelectGroup(action.groupName);
    }
  };

  const handleDragEnd = (result) => {
    if (!result.destination || saving || itemSearchText.trim()) return;
    if (result.source.index === result.destination.index) return;
    const reordered = [...items];
    const [item] = reordered.splice(result.source.index, 1);
    reordered.splice(result.destination.index, 0, item);
    setDrafts((current) => ({ ...current, [mode]: reordered }));
  };

  const handleMoveItem = (itemKey, direction) => {
    if (saving) return;
    const keyField = isNodeMode ? 'id' : 'airportId';
    const index = items.findIndex((entry) => entry[keyField] === itemKey);
    if (index < 0) return;

    let targetIndex = index;
    if (direction === 'top') targetIndex = 0;
    else if (direction === 'up') targetIndex = Math.max(0, index - 1);
    else if (direction === 'down') targetIndex = Math.min(items.length - 1, index + 1);
    else if (direction === 'bottom') targetIndex = items.length - 1;

    if (targetIndex === index) return;
    const reordered = [...items];
    const [moved] = reordered.splice(index, 1);
    reordered.splice(targetIndex, 0, moved);
    setDrafts((current) => ({ ...current, [mode]: reordered }));
  };

  const handleSave = async (reset = false) => {
    if (reset && !hasSavedCurrentMode && isCurrentModeDirty) {
      setDrafts((current) => ({ ...current, [mode]: undefined }));
      return;
    }

    setSaving(reset ? 'reset' : 'save');
    try {
      const order = reset ? [] : items;
      if (isNodeMode) {
        await saveGroupNodeOrder(
          selectedGroup,
          order.map((node) => node.id)
        );
      } else {
        await saveGroupAirportSort({
          groupName: selectedGroup,
          airportSorts: order.map((airport, sort) => ({ airportId: airport.airportId, sort }))
        });
      }
      setDrafts((current) => ({ ...current, [mode]: undefined }));
      setLoadStatus('loading');
      setDetailVersion((version) => version + 1);
      handleSnackbar(t(reset ? 'subscriptions.sort.messages.resetSuccess' : 'subscriptions.sort.messages.saveSuccess'), 'success');
      loadGroups();
    } catch {
      handleSnackbar(t(reset ? 'subscriptions.sort.messages.resetFailed' : 'subscriptions.sort.messages.saveFailed'), 'error');
    } finally {
      setSaving('');
    }
  };

  const filteredGroups = groups.filter((g) => !searchText || g.groupName.toLowerCase().includes(searchText.toLowerCase()));
  const trimmedItemSearch = itemSearchText.trim().toLowerCase();
  const isItemFiltered = trimmedItemSearch.length > 0;

  const indexedVisibleItems = useMemo(() => {
    return items
      .map((item, fullIndex) => ({ item, fullIndex }))
      .filter(({ item }) => {
        if (!trimmedItemSearch) return true;
        if (isNodeMode) {
          return (
            item.name?.toLowerCase().includes(trimmedItemSearch) ||
            item.airportName?.toLowerCase().includes(trimmedItemSearch) ||
            item.protocol?.toLowerCase().includes(trimmedItemSearch)
          );
        }
        return item.airportName?.toLowerCase().includes(trimmedItemSearch);
      });
  }, [items, trimmedItemSearch, isNodeMode]);

  const isSaveDisabled = loadStatus !== 'ready' || Boolean(saving) || items.length <= 1 || !isCurrentModeDirty;
  const isResetDisabled = loadStatus !== 'ready' || Boolean(saving) || items.length === 0 || (!hasSavedCurrentMode && !isCurrentModeDirty);

  const listItemHoverSurface = withAlpha(palette.primary.main, isDark ? 0.12 : 0.06);
  const listItemSelectedSurface = withAlpha(palette.primary.main, isDark ? 0.18 : 0.1);
  const listItemSelectedHoverSurface = withAlpha(palette.primary.main, isDark ? 0.24 : 0.14);
  const listItemSelectedBorder = withAlpha(palette.primary.main, isDark ? 0.38 : 0.24);
  const listItemHoverBorder = withAlpha(palette.primary.main, isDark ? 0.2 : 0.12);
  const listItemDivider = isDark ? withAlpha(palette.divider, 0.52) : panelBorder;
  const subtleBorder = withAlpha(palette.divider, isDark ? 0.68 : 0.86);
  const neutralSurface = isDark ? withAlpha(palette.background.default, 0.76) : palette.background.paper;
  const searchInsetHighlight = isDark ? `inset 0 1px 0 ${withAlpha(palette.common.white, 0.03)}` : 'none';
  const listShellSx = {
    height: isMobile ? 'auto' : '100%',
    display: 'flex',
    flexDirection: 'column',
    borderRadius: 3,
    overflow: 'hidden',
    bgcolor: nestedPanelSurface,
    border: '1px solid',
    borderColor: panelBorder,
    boxShadow: searchInsetHighlight
  };
  const sectionHeaderSx = {
    p: 2,
    bgcolor: mutedPanelSurface,
    borderBottom: '1px solid',
    borderColor: panelBorder,
    boxShadow: `inset 0 -1px 0 ${withAlpha(palette.divider, 0.34)}`
  };
  const searchFieldSx = {
    '& .MuiOutlinedInput-root': {
      borderRadius: 2.5,
      bgcolor: mutedPanelSurface,
      border: '1px solid',
      borderColor: panelBorder,
      boxShadow: searchInsetHighlight,
      transition: 'border-color 0.2s ease, background-color 0.2s ease, box-shadow 0.2s ease',
      '& fieldset': { borderColor: 'transparent' },
      '&:hover': {
        bgcolor: isDark ? withAlpha(palette.background.default, 0.92) : nestedPanelSurface,
        '& fieldset': {
          borderColor: withAlpha(palette.primary.main, isDark ? 0.28 : 0.18)
        }
      },
      '&.Mui-focused': {
        bgcolor: dialogSurface,
        boxShadow: isDark ? `0 0 0 1px ${withAlpha(palette.primary.main, 0.16)}` : 'none',
        '& fieldset': {
          borderColor: withAlpha(palette.primary.main, isDark ? 0.38 : 0.24)
        }
      }
    },
    '& .MuiInputBase-input::placeholder': {
      color: isDark ? withAlpha(palette.text.secondary, 0.92) : undefined,
      opacity: 1
    }
  };
  const dragRowSurface = withAlpha(palette.primary.main, isDark ? 0.12 : 0.06);
  const draggingSurface = withAlpha(palette.primary.main, isDark ? 0.18 : 0.1);
  const draggingBorder = withAlpha(palette.primary.main, isDark ? 0.42 : 0.28);
  const indexChipSx = {
    minWidth: 32,
    fontWeight: 700,
    bgcolor: withAlpha(palette.primary.main, isDark ? 0.14 : 0.08),
    color: palette.primary.main,
    border: '1px solid',
    borderColor: withAlpha(palette.primary.main, isDark ? 0.28 : 0.18)
  };
  const metaChipSx = {
    height: 20,
    fontSize: '0.7rem',
    color: secondaryText,
    bgcolor: withAlpha(palette.text.primary, isDark ? 0.08 : 0.05),
    border: '1px solid',
    borderColor: subtleBorder
  };
  const quickActionBtnSx = {
    color: secondaryText,
    p: 0.5,
    '&:hover': {
      color: primaryText,
      bgcolor: withAlpha(palette.primary.main, isDark ? 0.14 : 0.08)
    },
    '&.Mui-disabled': {
      color: withAlpha(palette.text.primary, isDark ? 0.24 : 0.26)
    }
  };

  const renderTabLabel = (label, isConfigured, isUnsaved) => (
    <Stack direction="row" alignItems="center" spacing={0.75}>
      <span>{label}</span>
      {isConfigured && (
        <Chip
          size="small"
          label={t('subscriptions.sort.savedBadge')}
          color="success"
          variant="outlined"
          sx={{ height: 18, fontSize: '0.68rem', '& .MuiChip-label': { px: 0.75 } }}
        />
      )}
      {isUnsaved && (
        <Chip
          size="small"
          label={t('subscriptions.sort.unsaved')}
          color="warning"
          variant="outlined"
          sx={{ height: 18, fontSize: '0.68rem', '& .MuiChip-label': { px: 0.75 } }}
        />
      )}
    </Stack>
  );

  const renderSortableItem = (item, fullIndex, { ref, draggableProps, dragHandleProps, isDragging = false } = {}) => {
    const itemKey = isNodeMode ? item.id : item.airportId;
    const isFirst = fullIndex === 0;
    const isLast = fullIndex === items.length - 1;
    return (
      <ListItem
        key={`${mode}-${itemKey}`}
        ref={ref}
        {...draggableProps}
        {...dragHandleProps}
        disableGutters
        sx={{
          mx: 1,
          mt: 1,
          mb: 0,
          px: isMobile ? 1.25 : 2,
          py: 1.25,
          width: 'auto',
          bgcolor: isDragging ? draggingSurface : neutralSurface,
          border: '1px solid',
          borderColor: isDragging ? draggingBorder : subtleBorder,
          borderRadius: 2,
          transition: 'background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease',
          ...(isDragging && {
            boxShadow: theme.shadows[6],
            transform: 'rotate(0.25deg)'
          }),
          '&:hover': {
            bgcolor: isDragging ? draggingSurface : dragRowSurface,
            borderColor: isDragging ? draggingBorder : listItemHoverBorder
          }
        }}
      >
        <Stack direction="row" alignItems="center" spacing={isMobile ? 1 : 1.5} sx={{ width: '100%', minWidth: 0 }}>
          <DragIndicatorIcon
            sx={{
              color: isItemFiltered ? tertiaryText : secondaryText,
              cursor: isItemFiltered ? 'default' : 'grab',
              opacity: isItemFiltered ? 0.45 : 1,
              flexShrink: 0
            }}
          />
          <Chip label={fullIndex + 1} size="small" sx={{ ...indexChipSx, flexShrink: 0 }} />
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack direction="row" alignItems="center" spacing={0.75} sx={{ minWidth: 0 }}>
              <Typography variant="body2" sx={{ color: primaryText, fontWeight: 600 }} noWrap>
                {isNodeMode ? item.name : item.airportName}
              </Typography>
              {isNodeMode && currentDetail?.hasNodeOrder && !item.inSavedOrder && (
                <Chip
                  label={t('subscriptions.sort.newNodeBadge')}
                  size="small"
                  color="info"
                  variant="outlined"
                  sx={{ height: 18, fontSize: '0.68rem', flexShrink: 0, '& .MuiChip-label': { px: 0.75 } }}
                />
              )}
            </Stack>
            {isNodeMode && (item.airportName || item.protocol) && (
              <Stack direction="row" alignItems="center" spacing={0.75} sx={{ mt: 0.5, flexWrap: 'wrap', gap: 0.5 }}>
                {item.airportName && <Chip label={item.airportName} size="small" sx={metaChipSx} />}
                {item.protocol && <Chip label={String(item.protocol).toUpperCase()} size="small" sx={metaChipSx} />}
              </Stack>
            )}
          </Box>
          {!isNodeMode && (
            <Chip
              label={t('subscriptions.sort.nodeCount', { count: item.nodeCount })}
              size="small"
              variant="outlined"
              color="primary"
              sx={{
                flexShrink: 0,
                bgcolor: withAlpha(palette.primary.main, isDark ? 0.1 : 0.05),
                borderColor: withAlpha(palette.primary.main, isDark ? 0.28 : 0.18)
              }}
            />
          )}
          {items.length > 1 && (
            <Stack
              direction="row"
              alignItems="center"
              spacing={0.25}
              onMouseDown={(e) => e.stopPropagation()}
              onTouchStart={(e) => e.stopPropagation()}
              sx={{ flexShrink: 0 }}
            >
              <Tooltip title={t('subscriptions.sort.moveTop')}>
                <span>
                  <IconButton
                    size="small"
                    disabled={Boolean(saving) || isFirst}
                    onClick={() => handleMoveItem(itemKey, 'top')}
                    aria-label={t('subscriptions.sort.moveTop')}
                    sx={quickActionBtnSx}
                  >
                    <VerticalAlignTopIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title={t('subscriptions.sort.moveUp')}>
                <span>
                  <IconButton
                    size="small"
                    disabled={Boolean(saving) || isFirst}
                    onClick={() => handleMoveItem(itemKey, 'up')}
                    aria-label={t('subscriptions.sort.moveUp')}
                    sx={quickActionBtnSx}
                  >
                    <KeyboardArrowUpIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title={t('subscriptions.sort.moveDown')}>
                <span>
                  <IconButton
                    size="small"
                    disabled={Boolean(saving) || isLast}
                    onClick={() => handleMoveItem(itemKey, 'down')}
                    aria-label={t('subscriptions.sort.moveDown')}
                    sx={quickActionBtnSx}
                  >
                    <KeyboardArrowDownIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title={t('subscriptions.sort.moveBottom')}>
                <span>
                  <IconButton
                    size="small"
                    disabled={Boolean(saving) || isLast}
                    onClick={() => handleMoveItem(itemKey, 'bottom')}
                    aria-label={t('subscriptions.sort.moveBottom')}
                    sx={quickActionBtnSx}
                  >
                    <VerticalAlignBottomIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
          )}
        </Stack>
      </ListItem>
    );
  };

  return (
    <>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="md"
        fullWidth
        fullScreen={isMobile}
        slotProps={{
          paper: {
            sx: {
              borderRadius: isMobile ? 0 : 3,
              overflow: 'hidden',
              m: isMobile ? 0 : 3,
              bgcolor: dialogSurface,
              backgroundImage: dialogSurfaceGradient,
              border: '1px solid',
              borderColor: panelBorder
            }
          }
        }}
      >
        <DialogTitle sx={sectionHeaderSx}>
          <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2} sx={{ flexWrap: 'wrap', gap: 1 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography variant="h6" sx={{ color: primaryText, fontWeight: 700 }}>
                {t('subscriptions.sort.groupSort')}
              </Typography>
              {hasAnyUnsavedChanges && (
                <Chip size="small" color="warning" variant="outlined" label={t('subscriptions.sort.unsaved')} sx={{ fontWeight: 600 }} />
              )}
            </Stack>
            {selectedGroup && (
              <Stack direction="row" spacing={1} alignItems="center">
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<RestartAltIcon />}
                  onClick={() => handleSave(true)}
                  disabled={isResetDisabled}
                  sx={{ borderColor: panelBorder, color: secondaryText }}
                >
                  {saving === 'reset' ? t('subscriptions.sort.resetting') : t('subscriptions.sort.resetSort')}
                </Button>
                <Button variant="contained" size="small" startIcon={<SaveIcon />} onClick={() => handleSave()} disabled={isSaveDisabled}>
                  {saving === 'save' ? t('subscriptions.sort.saving') : t('subscriptions.sort.saveSort')}
                </Button>
              </Stack>
            )}
          </Stack>
        </DialogTitle>

        <DialogContent sx={{ p: 2, bgcolor: dialogSurface }}>
          <Typography variant="caption" sx={{ display: 'block', mb: 1.5, color: tertiaryText }}>
            {t('subscriptions.sort.groupSortDesc')}
          </Typography>

          <Grid container spacing={2} sx={{ height: isMobile ? 'auto' : 'calc(70vh - 120px)' }}>
            <Grid item xs={12} md={4}>
              <Paper variant="outlined" sx={listShellSx}>
                <Box sx={{ ...sectionHeaderSx, p: 1.5 }}>
                  <TextField
                    fullWidth
                    size="small"
                    placeholder={t('subscriptions.sort.searchGroup')}
                    value={searchText}
                    onChange={(e) => setSearchText(e.target.value)}
                    sx={searchFieldSx}
                    slotProps={{
                      input: {
                        startAdornment: (
                          <InputAdornment position="start">
                            <SearchIcon fontSize="small" sx={{ color: secondaryText }} />
                          </InputAdornment>
                        ),
                        endAdornment: searchText && (
                          <InputAdornment position="end" sx={{ cursor: 'pointer', color: secondaryText }} onClick={() => setSearchText('')}>
                            <ClearIcon fontSize="small" />
                          </InputAdornment>
                        )
                      }
                    }}
                  />
                </Box>
                <List sx={{ overflow: 'auto', flex: 1, py: 0, maxHeight: isMobile ? 200 : 'none', bgcolor: nestedPanelSurface }} dense>
                  {filteredGroups.length === 0 ? (
                    <Box sx={{ p: 3, textAlign: 'center' }}>
                      <Typography variant="body2" sx={{ color: secondaryText }}>
                        {groups.length === 0 ? t('subscriptions.sort.noGroups') : t('common.noMatch')}
                      </Typography>
                    </Box>
                  ) : (
                    filteredGroups.map((group) => (
                      <ListItem key={group.groupName} disablePadding divider sx={{ borderColor: listItemDivider }}>
                        <ListItemButton
                          selected={selectedGroup === group.groupName}
                          onClick={() => handleGroupClick(group.groupName)}
                          disabled={Boolean(saving)}
                          sx={{
                            alignItems: 'flex-start',
                            px: 2,
                            py: 1.75,
                            color: primaryText,
                            borderLeft: '2px solid transparent',
                            transition: 'background-color 0.2s ease, border-color 0.2s ease',
                            '& .MuiTypography-root': {
                              color: 'inherit'
                            },
                            '& .MuiListItemText-primary': {
                              color: primaryText,
                              fontWeight: selectedGroup === group.groupName ? 700 : 600
                            },
                            '& .MuiListItemText-secondary': {
                              color: selectedGroup === group.groupName ? secondaryText : tertiaryText
                            },
                            '&:hover': {
                              bgcolor: listItemHoverSurface,
                              borderLeftColor: listItemHoverBorder,
                              color: primaryText,
                              '& .MuiListItemText-primary': {
                                color: primaryText
                              },
                              '& .MuiListItemText-secondary': {
                                color: secondaryText
                              }
                            },
                            '&.Mui-selected': {
                              bgcolor: listItemSelectedSurface,
                              borderLeftColor: listItemSelectedBorder,
                              color: primaryText,
                              '& .MuiListItemText-primary': {
                                color: primaryText
                              },
                              '& .MuiListItemText-secondary': {
                                color: secondaryText
                              }
                            },
                            '&.Mui-selected:hover': {
                              bgcolor: listItemSelectedHoverSurface,
                              color: primaryText,
                              '& .MuiListItemText-primary': {
                                color: primaryText
                              },
                              '& .MuiListItemText-secondary': {
                                color: secondaryText
                              }
                            }
                          }}
                        >
                          <ListItemText
                            primary={
                              <Stack direction="row" alignItems="center" spacing={1}>
                                <Typography variant="body2" noWrap sx={{ flex: 1, color: 'inherit', fontWeight: 'inherit' }}>
                                  {group.groupName}
                                </Typography>
                                {group.hasSortConfig && <CheckCircleIcon sx={{ fontSize: 16, color: 'success.main' }} />}
                              </Stack>
                            }
                            secondary={t('subscriptions.sort.airportNodeCount', {
                              airportCount: group.airportCount,
                              nodeCount: group.nodeCount
                            })}
                          />
                        </ListItemButton>
                      </ListItem>
                    ))
                  )}
                </List>
              </Paper>
            </Grid>

            <Grid item xs={12} md={8}>
              <Paper variant="outlined" sx={listShellSx}>
                {!selectedGroup ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1, p: 4 }}>
                    <Typography variant="body1" sx={{ color: secondaryText }}>
                      {t('subscriptions.sort.selectGroupPrompt')}
                    </Typography>
                  </Box>
                ) : loadStatus === 'loading' ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
                    <CircularProgress />
                  </Box>
                ) : loadStatus === 'error' ? (
                  <Alert
                    severity="error"
                    action={<Button onClick={() => setDetailVersion((version) => version + 1)}>{t('common.refresh')}</Button>}
                  >
                    {t('subscriptions.sort.messages.loadGroupDetailFailed')}
                  </Alert>
                ) : items.length === 0 ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1, p: 4 }}>
                    <Typography variant="body1" sx={{ color: secondaryText }}>
                      {t(isNodeMode ? 'subscriptions.sort.noNodesInGroup' : 'subscriptions.sort.noAirportsInGroup')}
                    </Typography>
                  </Box>
                ) : (
                  <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                    <Box sx={sectionHeaderSx}>
                      <Typography variant="subtitle1" sx={{ fontWeight: 700, color: primaryText }}>
                        {selectedGroup}
                      </Typography>
                      <Tabs
                        value={mode}
                        onChange={(_, value) => {
                          setMode(value);
                          setItemSearchText('');
                        }}
                        aria-label={t('subscriptions.sort.groupSort')}
                        sx={{ minHeight: 40, mt: 0.5 }}
                      >
                        <Tab
                          value="nodes"
                          label={renderTabLabel(t('subscriptions.sort.byNode'), Boolean(currentDetail?.hasNodeOrder), hasNodeChanges)}
                          disabled={Boolean(saving)}
                          sx={{ minHeight: 40, py: 0.75 }}
                        />
                        <Tab
                          value="airports"
                          label={renderTabLabel(
                            t('subscriptions.sort.byAirport'),
                            Boolean(currentDetail?.hasAirportSort),
                            hasAirportChanges
                          )}
                          disabled={Boolean(saving)}
                          sx={{ minHeight: 40, py: 0.75 }}
                        />
                      </Tabs>
                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5, color: secondaryText }}>
                        {t(isNodeMode ? 'subscriptions.sort.dragNodeHint' : 'subscriptions.sort.dragAirportHint')}
                      </Typography>
                      {!isNodeMode && currentDetail?.hasNodeOrder && (
                        <Alert severity="info" sx={{ mt: 1, py: 0.25 }}>
                          {t('subscriptions.sort.nodeOrderPrecedenceHint')}
                        </Alert>
                      )}
                      {items.length > 1 && (
                        <TextField
                          fullWidth
                          size="small"
                          placeholder={t(isNodeMode ? 'subscriptions.sort.searchNodes' : 'subscriptions.sort.searchAirports')}
                          value={itemSearchText}
                          onChange={(e) => setItemSearchText(e.target.value)}
                          sx={{ ...searchFieldSx, mt: 1.25 }}
                          slotProps={{
                            input: {
                              startAdornment: (
                                <InputAdornment position="start">
                                  <SearchIcon fontSize="small" sx={{ color: secondaryText }} />
                                </InputAdornment>
                              ),
                              endAdornment: itemSearchText && (
                                <InputAdornment
                                  position="end"
                                  sx={{ cursor: 'pointer', color: secondaryText }}
                                  onClick={() => setItemSearchText('')}
                                >
                                  <ClearIcon fontSize="small" />
                                </InputAdornment>
                              )
                            }
                          }}
                        />
                      )}
                      {isItemFiltered && (
                        <Typography variant="caption" sx={{ display: 'block', mt: 0.75, color: tertiaryText }}>
                          {t('subscriptions.sort.dragDisabledWhileFiltering')}
                        </Typography>
                      )}
                    </Box>

                    <Box sx={{ flex: 1, overflow: 'auto', bgcolor: nestedPanelSurface, pb: 1 }}>
                      {indexedVisibleItems.length === 0 ? (
                        <Box sx={{ p: 4, textAlign: 'center' }}>
                          <Typography variant="body2" sx={{ color: secondaryText }}>
                            {t('common.noMatch')}
                          </Typography>
                        </Box>
                      ) : isItemFiltered ? (
                        <List sx={{ py: 0, bgcolor: nestedPanelSurface }}>
                          {indexedVisibleItems.map(({ item, fullIndex }) => renderSortableItem(item, fullIndex))}
                        </List>
                      ) : (
                        <DragDropContext onDragEnd={handleDragEnd}>
                          <Droppable droppableId={mode}>
                            {(provided) => (
                              <List {...provided.droppableProps} ref={provided.innerRef} sx={{ py: 0, bgcolor: nestedPanelSurface }}>
                                {indexedVisibleItems.map(({ item, fullIndex }) => {
                                  const itemKey = isNodeMode ? item.id : item.airportId;
                                  return (
                                    <Draggable
                                      key={`${mode}-${itemKey}`}
                                      draggableId={`${mode}-${itemKey}`}
                                      index={fullIndex}
                                      isDragDisabled={Boolean(saving)}
                                    >
                                      {(provided, snapshot) =>
                                        renderSortableItem(item, fullIndex, {
                                          ref: provided.innerRef,
                                          draggableProps: provided.draggableProps,
                                          dragHandleProps: provided.dragHandleProps,
                                          isDragging: snapshot.isDragging
                                        })
                                      }
                                    </Draggable>
                                  );
                                })}
                                {provided.placeholder}
                              </List>
                            )}
                          </Droppable>
                        </DragDropContext>
                      )}
                    </Box>
                  </Box>
                )}
              </Paper>
            </Grid>
          </Grid>
        </DialogContent>

        <DialogActions
          sx={{
            px: 2,
            py: 1.5,
            bgcolor: mutedPanelSurface,
            borderTop: '1px solid',
            borderColor: panelBorder
          }}
        >
          <Button
            onClick={handleClose}
            disabled={Boolean(saving)}
            variant="outlined"
            sx={{ borderColor: panelBorder, color: secondaryText }}
          >
            {t('common.close')}
          </Button>
        </DialogActions>
      </Dialog>

      <ConfirmDialog
        open={Boolean(pendingAction)}
        title={t('subscriptions.sort.unsavedTitle')}
        content={t('subscriptions.sort.unsavedConfirm')}
        onClose={() => setPendingAction(null)}
        onConfirm={handleConfirmDiscard}
      />

      {!showMessage && (
        <Snackbar
          open={snackbar.open}
          autoHideDuration={3000}
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        >
          <Alert onClose={() => setSnackbar({ ...snackbar, open: false })} severity={snackbar.severity} sx={{ width: '100%' }}>
            {snackbar.message}
          </Alert>
        </Snackbar>
      )}
    </>
  );
}
