// Copyright 2026 Matt Miller

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// [http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import React, { useState, useMemo } from 'react';
import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  Chip,
  IconButton,
  Tooltip,
  Typography,
  Stack,
  Collapse,
  Box,
  Skeleton,
  Alert,
  Button,
} from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import { ContainerInfo } from '../types';
import { CONTAINER_ID_DISPLAY_LENGTH } from '../constants';
import { cleanContainerName } from '../utils/containerUtils';

interface ContainersTableProps {
  containers: ContainerInfo[];
  isLoading: boolean;
  onCopyToClipboard: (text: string, label: string) => void;
  proxyUnreachable?: boolean;
  onProxyHelp?: () => void;
}

export function ContainersTable({
  containers,
  isLoading,
  onCopyToClipboard,
  proxyUnreachable,
  onProxyHelp,
}: ContainersTableProps) {
  const [expandedContainer, setExpandedContainer] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState<'name' | 'id'>('name');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');

  const handleSort = (column: 'name' | 'id') => {
    const isAsc = sortBy === column && sortOrder === 'asc';
    setSortOrder(isAsc ? 'desc' : 'asc');
    setSortBy(column);
  };

  const sortedContainers = useMemo(() => {
    return [...containers].sort((a, b) => {
      const aValue = sortBy === 'name'
        ? cleanContainerName(a.name).toLowerCase()
        : a.containerId;
      const bValue = sortBy === 'name'
        ? cleanContainerName(b.name).toLowerCase()
        : b.containerId;
      return sortOrder === 'asc' ? aValue.localeCompare(bValue) : bValue.localeCompare(aValue);
    });
  }, [containers, sortBy, sortOrder]);

  const handleRowClick = (containerId: string) => {
    setExpandedContainer((prev) => (prev === containerId ? null : containerId));
  };

  const handleCopyClick = (text: string, label: string, event: React.MouseEvent) => {
    event.stopPropagation(); // Prevent row expansion when clicking copy button
    onCopyToClipboard(text, label);
  };

  const renderContent = () => {
    if (isLoading && containers.length === 0) {
      return (
        <Stack spacing={1}>
          <Skeleton variant="rectangular" height={40} />
          <Skeleton variant="rectangular" height={40} />
          <Skeleton variant="rectangular" height={40} />
        </Stack>
      );
    }

    if (!isLoading && containers.length === 0) {
      return (
        <Box
          sx={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            py: 4,
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 1,
          }}
        >
          <Typography variant="body2" color="text.secondary">
            No labeled containers are running.
          </Typography>
        </Box>
      );
    }

    return (
      <TableContainer component={Paper} variant="outlined" sx={{ flex: 1, overflow: 'auto' }}>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>
                <TableSortLabel
                  active={sortBy === 'name'}
                  direction={sortBy === 'name' ? sortOrder : 'asc'}
                  onClick={() => handleSort('name')}
                >
                  Name
                </TableSortLabel>
              </TableCell>
              <TableCell>
                <TableSortLabel
                  active={sortBy === 'id'}
                  direction={sortBy === 'id' ? sortOrder : 'asc'}
                  onClick={() => handleSort('id')}
                >
                  Container ID
                </TableSortLabel>
              </TableCell>
              <TableCell>Networks</TableCell>
              <TableCell padding="checkbox" />
            </TableRow>
          </TableHead>
          <TableBody>
            {sortedContainers.map((container) => {
              const displayName = cleanContainerName(container.name);
              const isExpanded = expandedContainer === container.containerId;
              const sortedLabels = Object.entries(container.labels).sort(([a], [b]) =>
                a.localeCompare(b),
              );
              return (
                <React.Fragment key={container.containerId}>
                  <TableRow
                    hover
                    selected={isExpanded}
                    onClick={() => handleRowClick(container.containerId)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        if (event.target === event.currentTarget) {
                          event.preventDefault();
                          handleRowClick(container.containerId);
                        }
                      }
                    }}
                    tabIndex={0}
                    aria-expanded={isExpanded}
                    sx={{
                      cursor: 'pointer',
                      '& .copy-button': {
                        opacity: 0,
                        pointerEvents: 'none',
                        transition: 'opacity 120ms ease-in-out',
                      },
                      '&:hover .copy-button': {
                        opacity: 1,
                        pointerEvents: 'auto',
                      },
                      '&:focus-within .copy-button': {
                        opacity: 1,
                        pointerEvents: 'auto',
                      },
                    }}
                  >
                    <TableCell>
                      <Stack direction="row" alignItems="center" spacing={0.5}>
                        <Typography variant="body1">{displayName}</Typography>
                        <Tooltip title="Copy name">
                          <IconButton
                            size="small"
                            aria-label={`Copy container name ${displayName}`}
                            onClick={(e) => handleCopyClick(displayName, 'container name', e)}
                            className="copy-button"
                            sx={{ padding: '2px' }}
                          >
                            <ContentCopyIcon fontSize="inherit" sx={{ fontSize: '0.75rem' }} />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Stack direction="row" alignItems="center" spacing={0.5}>
                        <Typography
                          variant="body1"
                          sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                        >
                          {container.containerId.substring(0, CONTAINER_ID_DISPLAY_LENGTH)}
                        </Typography>
                        <Tooltip title="Copy full ID">
                          <IconButton
                            size="small"
                            aria-label={`Copy container id ${container.containerId}`}
                            onClick={(e) =>
                              handleCopyClick(container.containerId, 'container ID', e)
                            }
                            className="copy-button"
                            sx={{ padding: '2px' }}
                          >
                            <ContentCopyIcon fontSize="inherit" sx={{ fontSize: '0.75rem' }} />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                        {container.addresses.map((a) => (
                          <Tooltip
                            key={a.ip}
                            title={a.connected ? 'Connected' : 'Not connected'}
                          >
                            <Chip
                              label={a.ip}
                              color={a.connected ? 'success' : 'default'}
                              size="small"
                              variant="outlined"
                              tabIndex={0}
                              sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}
                            />
                          </Tooltip>
                        ))}
                      </Stack>
                    </TableCell>
                    <TableCell padding="checkbox">
                      <IconButton
                        size="small"
                        aria-label={isExpanded ? 'Collapse labels' : 'Expand labels'}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRowClick(container.containerId);
                        }}
                      >
                        {isExpanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                      </IconButton>
                    </TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell colSpan={4} sx={{ pb: 0, pt: 0, borderBottom: isExpanded ? undefined : 'none' }}>
                      <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                        <Box sx={{ py: 1.5, px: 1 }}>
                          <Typography variant="subtitle2" gutterBottom>
                            Labels
                          </Typography>
                          {sortedLabels.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                              No labels
                            </Typography>
                          ) : (
                            <Stack spacing={0.25}>
                              {sortedLabels.map(([key, value]) => (
                                <Stack key={key} direction="row" spacing={2} alignItems="baseline">
                                  <Typography
                                    variant="body2"
                                    sx={{
                                      fontFamily: 'monospace',
                                      color: 'text.secondary',
                                      minWidth: 160,
                                      wordBreak: 'break-all',
                                    }}
                                  >
                                    {key}
                                  </Typography>
                                  <Typography variant="body2" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                                    {value}
                                  </Typography>
                                </Stack>
                              ))}
                            </Stack>
                          )}
                        </Box>
                      </Collapse>
                    </TableCell>
                  </TableRow>
                </React.Fragment>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
    );
  };

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
      <Typography variant="subtitle1" sx={{ mb: 1 }}>Enabled for these containers</Typography>
      {proxyUnreachable && (
        <Alert
          severity="warning"
          sx={{ mb: 1, alignItems: 'center' }}
          action={
            onProxyHelp && (
              <Button color="inherit" size="small" onClick={onProxyHelp}>
                Get help
              </Button>
            )
          }
        >
          Extension backend not responding - list may be outdated.
        </Alert>
      )}
      <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
        {renderContent()}
      </Box>
      <Stack direction="row" justifyContent="flex-end" alignItems="center" sx={{ mt: 0.5 }}>
        <Typography variant="body2" color="text.secondary">
          Showing {containers.length} {containers.length === 1 ? 'item' : 'items'}
        </Typography>
      </Stack>
    </Box>
  );
}
