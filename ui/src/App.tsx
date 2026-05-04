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

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { createDockerDesktopClient } from '@docker/extension-api-client';
import { useDockerDesktopService } from './services/dockerDesktopService';
import logo from './logo.svg';
import {
  CONTAINER_POLL_INTERVAL_MS,
  SNACKBAR_SUCCESS_DURATION_MS,
  GITHUB_REPO_URL,
  IMDS_PROXY_ENABLED_LABEL,
  BACKEND_REQUEST_TIMEOUT_MS,
} from './constants';

/* v8 ignore next 8 -- service is always mocked in tests */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    promise,
    new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error('Request timed out')), ms)
    ),
  ]);
}
import {
  ContainerInfo,
  ProxyContainerState,
  isContainersResponse,
} from './types';
import { ContainersTable } from './components/ContainersTable';
import { SettingsForm } from './components/SettingsForm';
import {
  Stack,
  Typography,
  Box,
  IconButton,
  Tooltip,
  Snackbar,
  Alert,
  Link,
  Tabs,
  Tab,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  List,
  ListItem,
  ListItemText,
} from '@mui/material';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

// Note: This line relies on Docker Desktop's presence as a host application.
// If you're running this React app in a browser, it won't work properly.
function useDockerDesktopClient() {
  const [client, setClient] = useState<ReturnType<typeof createDockerDesktopClient> | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    try {
      const createdClient = createDockerDesktopClient();
      setClient(createdClient);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to initialize Docker Desktop client';
      setError(message);
    }
  }, []);

  return { client, error };
}

export function App() {
  const { client: ddClient, error: clientError } = useDockerDesktopClient();
  const service = useDockerDesktopService(ddClient);

  const [activeTab, setActiveTab] = useState(0);
  const [proxyUnreachable, setProxyUnreachable] = useState(false);
  const [proxyHelpOpen, setProxyHelpOpen] = useState(false);
  const [proxyContainerState, setProxyContainerState] = useState<ProxyContainerState | null>(null);

  // Container state
  const [containers, setContainers] = useState<ContainerInfo[]>([]);
  const [isLoadingContainers, setIsLoadingContainers] = useState(false);

  // Snackbar notification state
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState('');
  const [snackbarSeverity, setSnackbarSeverity] = useState<'success' | 'error'>('success');

  // Track mount status to prevent state updates after unmount during async operations
  const isMountedRef = useRef(false);
  // Only show the loading skeleton on the first fetch, not on background poll refreshes
  const hasLoadedOnceRef = useRef(false);
  // Require 2 consecutive failures before showing the unreachable banner, to avoid
  // flicker when the controller is briefly unavailable during startup or restart
  const consecutiveFailuresRef = useRef(0);
  const UNREACHABLE_THRESHOLD = 2;

  const showSnackbar = useCallback((message: string, severity: 'success' | 'error') => {
    setSnackbarMessage(message);
    setSnackbarSeverity(severity);
    setSnackbarOpen(true);
  }, []);

  const loadContainers = useCallback(async () => {
    if (!ddClient || !isMountedRef.current) {
      return;
    }

    if (!hasLoadedOnceRef.current) {
      setIsLoadingContainers(true);
    }
    try {
      const result = await withTimeout(service.getContainers(), BACKEND_REQUEST_TIMEOUT_MS);
      if (isContainersResponse(result) && isMountedRef.current) {
        hasLoadedOnceRef.current = true;
        consecutiveFailuresRef.current = 0;
        setContainers(result.containers);
        setProxyContainerState(result.proxyStatus);
        setProxyUnreachable(false);
      } else if (isMountedRef.current) {
        showSnackbar('Unexpected containers response format', 'error');
      }
    } catch (error) {
      console.error('Failed to load containers:', error);
      consecutiveFailuresRef.current += 1;
      if (isMountedRef.current && consecutiveFailuresRef.current >= UNREACHABLE_THRESHOLD) {
        setProxyUnreachable(true);
      }
    } finally {
      // Mark as attempted regardless of outcome so we never show the skeleton again
      hasLoadedOnceRef.current = true;
      if (isMountedRef.current) {
        setIsLoadingContainers(false);
      }
    }
  }, [ddClient, service, showSnackbar]);

  useEffect(() => {
    if (clientError) {
      showSnackbar(clientError, 'error');
    }
  }, [clientError, showSnackbar]);

  useEffect(() => {
    if (!ddClient) {
      return;
    }
    isMountedRef.current = true;

    loadContainers();

    const interval = setInterval(loadContainers, CONTAINER_POLL_INTERVAL_MS);

    return () => {
      isMountedRef.current = false;
      clearInterval(interval);
    };
  }, [ddClient, loadContainers]);

  const copyToClipboard = (text: string, label: string) => {
    if (!navigator.clipboard || !navigator.clipboard.writeText) {
      showSnackbar('Clipboard not available', 'error');
      return;
    }

    navigator.clipboard.writeText(text)
      .then(() => {
        showSnackbar(`Copied ${label} to clipboard`, 'success');
      })
      .catch(err => {
        console.error('Failed to copy to clipboard:', err);
        showSnackbar('Failed to copy to clipboard', 'error');
      });
  };

  const handleProxyAction = useCallback(async () => {
    if (!ddClient || !proxyContainerState) return;
    try {
      if (proxyContainerState === 'paused') {
        await ddClient.docker.cli.exec('unpause', ['imds-proxy']);
      } else if (proxyContainerState === 'missing') {
        const { projectName, configFiles } = await service.getComposeProjectName();
        const args = configFiles
          ? ['-f', configFiles, '-p', projectName, 'up', '-d', 'imds-proxy']
          : ['-p', projectName, 'up', '-d', 'imds-proxy'];
        await ddClient.docker.cli.exec('compose', args);
      } else {
        await ddClient.docker.cli.exec('start', ['imds-proxy']);
      }
      loadContainers();
    } catch (err) {
      console.error('Failed to control proxy container:', err);
      showSnackbar('Failed to start proxy container', 'error');
    }
  }, [ddClient, proxyContainerState, service, loadContainers, showSnackbar]);

  const handleSnackbarClose = () => {
    setSnackbarOpen(false);
  };

  const handleOpenDocumentation = (event: React.MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    if (ddClient?.host?.openExternal) {
      ddClient.host.openExternal(GITHUB_REPO_URL);
      return;
    }

    window.open(GITHUB_REPO_URL, '_blank', 'noopener,noreferrer');
  };

  return (
    <Box sx={{ height: '100vh', overflow: 'hidden', display: 'flex', flexDirection: 'column', pt: 0, pr: 1, pb: 3, pl: 0 }}>
      {/* Header */}
      <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
        <Stack direction="row" spacing={2} alignItems="center">
          <Box
            component="img"
            src={logo}
            alt="Barnacle Logo"
            sx={{ width: 40, height: 40 }}
          />
          <Typography variant="h3">
            Barnacle IMDS Proxy
          </Typography>
        </Stack>
        <Link
          href={GITHUB_REPO_URL}
          onClick={handleOpenDocumentation}
          sx={{ display: 'flex', gap: 0.5, alignItems: 'center', textDecoration: 'none' }}
        >
          <OpenInNewIcon fontSize="small" />
          <Typography variant="body1">View documentation</Typography>
        </Link>
      </Stack>

      {/* Tabs */}
      <Tabs
        value={activeTab}
        onChange={(_e, newValue: number) => setActiveTab(newValue)}
        sx={{ mb: 1.5, borderBottom: 1, borderColor: 'divider' }}
      >
        <Tab label="Containers" />
        <Tab label="Settings" />
      </Tabs>

      {/* Containers tab */}
      {activeTab === 0 && (
        <Box sx={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
            <Typography variant="body2">
              Add the following label to a container to enable IMDS proxying:
            </Typography>
            <Tooltip title="Copy label">
              <Box
                component="code"
                onClick={() => copyToClipboard(IMDS_PROXY_ENABLED_LABEL, 'label')}
                aria-label="Copy label to clipboard"
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') copyToClipboard(IMDS_PROXY_ENABLED_LABEL, 'label');
                }}
                sx={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 0.5,
                  px: 1,
                  py: 0.25,
                  borderRadius: 0.5,
                  bgcolor: 'action.hover',
                  border: '1px solid',
                  borderColor: 'divider',
                  fontFamily: 'monospace',
                  fontSize: '0.875rem',
                  whiteSpace: 'nowrap',
                  cursor: 'pointer',
                  '&:hover': { bgcolor: 'action.selected' },
                }}
              >
                {IMDS_PROXY_ENABLED_LABEL}
                <ContentCopyIcon sx={{ fontSize: '0.75rem', opacity: 0.6 }} />
              </Box>
            </Tooltip>
          </Stack>

          {proxyContainerState !== null && proxyContainerState !== 'running' && (
            <Alert
              severity={proxyContainerState === 'paused' || proxyContainerState === 'stopped' ? 'warning' : 'error'}
              sx={{ mb: 1, alignItems: 'center' }}
              action={
                <Button color="inherit" size="small" onClick={handleProxyAction}>
                  {proxyContainerState === 'paused' ? 'Unpause' : 'Start'}
                </Button>
              }
            >
              {proxyContainerState === 'paused' && 'The IMDS proxy container is paused - IMDS requests are not being proxied.'}
              {proxyContainerState === 'stopped' && 'The IMDS proxy container has stopped - IMDS requests are not being proxied.'}
              {proxyContainerState === 'failed' && 'The IMDS proxy container has crashed - IMDS requests are not being proxied.'}
              {proxyContainerState === 'missing' && 'The IMDS proxy container is not running - IMDS requests are not being proxied.'}
            </Alert>
          )}

          <ContainersTable
            containers={containers}
            isLoading={isLoadingContainers}
            onCopyToClipboard={copyToClipboard}
            proxyUnreachable={proxyUnreachable}
            onProxyHelp={() => setProxyHelpOpen(true)}
          />
        </Box>
      )}

      {/* Settings tab */}
      {activeTab === 1 && (
        <Box sx={{ maxWidth: 600 }}>
          <SettingsForm
            ddClient={ddClient}
            service={service}
            showSnackbar={showSnackbar}
            proxyUnreachable={proxyUnreachable}
            onProxyHelp={() => setProxyHelpOpen(true)}
          />
        </Box>
      )}

      <Dialog open={proxyHelpOpen} onClose={() => setProxyHelpOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Extension backend not responding</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 1.5 }}>
            The Barnacle controller is not responding, so the container list may be outdated.
            The IMDS proxy runs separately and may still be forwarding traffic correctly.
            Try the following steps in order until it recovers:
          </Typography>
          <List dense disablePadding>
            <ListItem sx={{ alignItems: 'flex-start', pl: 0 }}>
              <ListItemText
                primary="1. Navigate away and return"
                secondary="Click another section in the Docker Desktop sidebar, then come back to this extension."
              />
            </ListItem>
            <ListItem sx={{ alignItems: 'flex-start', pl: 0 }}>
              <ListItemText
                primary="2. Disable and re-enable the extension"
                secondary="In the Extensions Marketplace, find Barnacle and click Disable, then Enable."
              />
            </ListItem>
            <ListItem sx={{ alignItems: 'flex-start', pl: 0 }}>
              <ListItemText
                primary="3. Restart Docker Desktop"
                secondary="Quit and relaunch Docker Desktop. This restarts all extension services."
              />
            </ListItem>
            <ListItem sx={{ alignItems: 'flex-start', pl: 0 }}>
              <ListItemText
                primary="4. Reboot"
                secondary={
                  <>
                    {"Still not working? Reboot your machine, then "}
                    <Link
                      href="#"
                      onClick={(e) => {
                        e.preventDefault();
                        if (ddClient?.host?.openExternal) {
                          ddClient.host.openExternal(`${GITHUB_REPO_URL}#troubleshooting`);
                        } else {
                          window.open(`${GITHUB_REPO_URL}#troubleshooting`, '_blank', 'noopener,noreferrer');
                        }
                      }}
                    >
                      view the troubleshooting guide
                    </Link>
                    {" if the problem persists."}
                  </>
                }
              />
            </ListItem>
          </List>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setProxyHelpOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={snackbarOpen}
        autoHideDuration={snackbarSeverity === 'success' ? SNACKBAR_SUCCESS_DURATION_MS : null}
        onClose={handleSnackbarClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={handleSnackbarClose} severity={snackbarSeverity} variant="standard">
          {snackbarMessage}
        </Alert>
      </Snackbar>
    </Box>
  );
}
