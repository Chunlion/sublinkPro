import { memo, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

// material-ui
import { useTheme, alpha } from '@mui/material/styles';
import Avatar from '@mui/material/Avatar';
import Card from '@mui/material/Card';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Tooltip from '@mui/material/Tooltip';
import IconButton from '@mui/material/IconButton';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';

// assets
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';

// project imports
import useConfig from 'hooks/useConfig';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getReadableTextTokens, getSurfaceTokens } from 'themes/surfaceTokens';
import { withAlpha } from 'utils/colorUtils';

// GitHub 仓库配置
const GITHUB_REPO = 'Chunlion/sublinkPro';
const GITHUB_URL = `https://github.com/${GITHUB_REPO}`;

// ==============================|| SIDEBAR - VERSION CARD ||============================== //

const getMenuCardTokens = (theme, isDark, statusColor) => {
  const { palette, dialogSurface, mutedPanelSurface, nestedPanelSurface, panelBorder } = getSurfaceTokens(theme, isDark);
  const { primaryText, secondaryText, tertiaryText } = getReadableTextTokens(theme, isDark);

  return {
    palette,
    primaryText,
    secondaryText,
    tertiaryText,
    emphasizedText: isDark ? withAlpha(primaryText, 0.98) : primaryText,
    cardSurface: isDark
      ? `linear-gradient(180deg, ${withAlpha(palette.background.paper, 0.96)} 0%, ${dialogSurface} 100%)`
      : `linear-gradient(180deg, ${withAlpha(palette.background.paper, 0.98)} 0%, ${withAlpha(palette.background.default, 0.9)} 100%)`,
    cardBorder: panelBorder,
    panelBorder,
    statusBorder: withAlpha(statusColor, isDark ? 0.28 : 0.2),
    currentVersionSurface: isDark
      ? `linear-gradient(180deg, ${withAlpha(nestedPanelSurface, 0.66)} 0%, ${withAlpha(mutedPanelSurface, 0.96)} 100%)`
      : withAlpha(palette.background.default, 0.78),
    currentVersionBorder: isDark ? withAlpha(palette.divider, 0.76) : withAlpha(palette.divider, 0.76),
    statusPanelSurface: isDark
      ? `linear-gradient(180deg, ${withAlpha(statusColor, 0.14)} 0%, ${withAlpha(mutedPanelSurface, 0.96)} 100%)`
      : withAlpha(statusColor, 0.06),
    iconButtonBackground: isDark ? withAlpha(nestedPanelSurface, 0.42) : withAlpha(palette.background.paper, 0.98),
    iconButtonHoverBackground: isDark ? withAlpha(nestedPanelSurface, 0.58) : withAlpha(theme.palette.primary.main, 0.08),
    titleColor: isDark ? withAlpha(primaryText, 0.96) : primaryText,
    mutedTextColor: isDark ? withAlpha(primaryText, 0.72) : secondaryText,
    statusTextColor: isDark ? withAlpha(primaryText, 0.88) : primaryText,
    statusHintColor: isDark ? withAlpha(primaryText, 0.72) : secondaryText
  };
};

function MenuCard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isDark } = useResolvedColorScheme();
  const palette = theme.vars?.palette || theme.palette;
  const { version } = useConfig();

  const currentVersion = version || 'custom';

  const versionStatus = useMemo(() => {
    return {
      key: 'current',
      label: t('version.status.current'),
      tone: 'success',
      hint: t('version.hint.customBuild')
    };
  }, [t]);

  const statusToneMap = {
    success: theme.palette.success.main,
    default: isDark ? withAlpha(palette.text.primary, 0.72) : palette.text.secondary
  };

  const statusColor = statusToneMap[versionStatus.tone] || statusToneMap.default;
  const {
    cardSurface,
    cardBorder,
    panelBorder,
    statusBorder,
    mutedTextColor,
    statusTextColor,
    emphasizedText,
    statusHintColor,
    statusPanelSurface,
    currentVersionSurface,
    currentVersionBorder,
    titleColor,
    iconButtonBackground,
    iconButtonHoverBackground
  } = getMenuCardTokens(theme, isDark, statusColor);
  const titleAccentColor = theme.palette.primary.main;
  const iconButtonColor = mutedTextColor;

  return (
    <Card
      sx={{
        background: cardSurface,
        mb: 2.75,
        overflow: 'hidden',
        position: 'relative',
        boxShadow: isDark
          ? `0 10px 24px ${alpha(theme.palette.common.black, 0.18)}, inset 0 1px 0 ${alpha(theme.palette.common.white, 0.04)}`
          : `0 1px 3px ${alpha(theme.palette.common.black, 0.05)}`,
        border: `1px solid ${cardBorder}`,
        '&:after': {
          content: '""',
          position: 'absolute',
          width: 120,
          height: 120,
          bgcolor: isDark ? alpha(theme.palette.primary.main, 0.04) : alpha(theme.palette.primary.main, 0.06),
          borderRadius: '50%',
          top: -72,
          right: -68
        }
      }}
    >
      <Box sx={{ p: 1.35, position: 'relative', zIndex: 1 }}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
          <Avatar
            variant="rounded"
            sx={{
              ...theme.typography.mediumAvatar,
              width: 32,
              height: 32,
              borderRadius: 2,
              color: titleAccentColor,
              border: '1px solid',
              borderColor: alpha(theme.palette.primary.main, isDark ? 0.28 : 0.16),
              bgcolor: isDark ? alpha(theme.palette.background.paper, 0.16) : alpha(theme.palette.primary.main, 0.06),
              boxShadow: 'none',
              flexShrink: 0
            }}
          >
            <InfoOutlinedIcon fontSize="small" />
          </Avatar>

          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 0.75 }}>
              <Box sx={{ minWidth: 0 }}>
                <Typography
                  variant="subtitle2"
                  sx={{
                    color: titleColor,
                    fontWeight: 700,
                    lineHeight: 1.2
                  }}
                >
                  SublinkPro
                </Typography>
                <Typography variant="caption" sx={{ display: 'block', color: mutedTextColor, mt: 0.15, lineHeight: 1.2 }}>
                  {t('version.currentSystemVersion')}
                </Typography>
              </Box>

              <Tooltip title={t('version.githubRepo')}>
                <IconButton
                  size="small"
                  component="a"
                  href={GITHUB_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={t('version.githubRepo')}
                  sx={{
                    p: 0.5,
                    color: iconButtonColor,
                    background: iconButtonBackground,
                    border: '1px solid',
                    borderColor: panelBorder,
                    '&:hover': {
                      background: iconButtonHoverBackground,
                      color: statusTextColor
                    }
                  }}
                >
                  <OpenInNewIcon sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            </Box>

            <Box
              sx={{
                mt: 0.7,
                px: 0.8,
                py: 0.65,
                borderRadius: 1.25,
                background: currentVersionSurface,
                border: '1px solid',
                borderColor: currentVersionBorder,
                boxShadow: isDark ? `inset 0 1px 0 ${alpha(theme.palette.common.white, 0.02)}` : 'none'
              }}
            >
              <Typography
                variant="body2"
                sx={{
                  color: emphasizedText,
                  fontWeight: 700,
                  lineHeight: 1.2,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap'
                }}
              >
                {currentVersion}
              </Typography>
            </Box>
          </Box>
        </Box>

        <Box
          sx={{
            mt: 0.8,
            p: 0.9,
            borderRadius: 1.5,
            background: statusPanelSurface,
            border: '1px solid',
            borderColor: statusBorder,
            boxShadow: isDark ? `inset 0 1px 0 ${alpha(theme.palette.common.white, 0.02)}` : 'none'
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 0.75, minWidth: 0 }}>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.65, minWidth: 0 }}>
                <Box
                  sx={{
                    width: 6,
                    height: 6,
                    borderRadius: '50%',
                    bgcolor: statusColor,
                    boxShadow: `0 0 0 3px ${withAlpha(statusColor, isDark ? 0.14 : 0.1)}`,
                    flexShrink: 0,
                    mt: 0.6
                  }}
                />
                <Box sx={{ minWidth: 0, flex: 1 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      color: statusTextColor,
                      fontWeight: 700,
                      lineHeight: 1.2,
                      whiteSpace: 'nowrap'
                    }}
                  >
                    {versionStatus.label}
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{
                      display: 'block',
                      mt: 0.2,
                      color: statusHintColor,
                      minWidth: 0,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap'
                    }}
                  >
                    {versionStatus.hint}
                  </Typography>
                </Box>
              </Box>
            </Box>

          </Box>
        </Box>
      </Box>
    </Card>
  );
}

export default memo(MenuCard);
