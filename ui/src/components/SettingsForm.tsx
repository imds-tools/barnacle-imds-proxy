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

import React, { useState, useEffect, useRef } from 'react';
import {
  Stack, TextField, Typography, Skeleton, Alert,
  Chip, Box, IconButton,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import Button from '@mui/material/Button';
import { createDockerDesktopClient } from '@docker/extension-api-client';
import { DockerDesktopServiceClient } from '../services/dockerDesktopService';
import { isSettingsResponse } from '../types';
import { SAVE_DEBOUNCE_MS, BACKEND_REQUEST_TIMEOUT_MS } from '../constants';

function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    promise,
    new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error('Request timed out')), ms)
    ),
  ]);
}

interface SettingsFormProps {
  ddClient: ReturnType<typeof createDockerDesktopClient> | null;
  service: DockerDesktopServiceClient;
  showSnackbar: (message: string, severity: 'success' | 'error') => void;
  proxyUnreachable?: boolean;
  onProxyHelp?: () => void;
}

export function SettingsForm({ ddClient, service, showSnackbar, proxyUnreachable, onProxyHelp }: SettingsFormProps) {
  const [url, setUrl] = useState('');
  const [urlError, setUrlError] = useState('');
  const [savedUrl, setSavedUrl] = useState('');
  const [customIPs, setCustomIPs] = useState<string[]>([]);
  const [savedCustomIPs, setSavedCustomIPs] = useState<string[]>([]);
  const [newIP, setNewIP] = useState('');
  const [ipError, setIpError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isLoadingSettings, setIsLoadingSettings] = useState(false);
  const [isDebouncing, setIsDebouncing] = useState(false);
  const [hasBeenSaved, setHasBeenSaved] = useState(false);

  // Track mount status to prevent state updates after unmount during async settings load
  const isMountedRef = useRef(false);
  const urlRef = useRef(url);
  const savedUrlRef = useRef(savedUrl);
  const customIPsRef = useRef(customIPs);
  const savedCustomIPsRef = useRef(savedCustomIPs);

  // Keep refs in sync so polling callbacks can read current values without stale closures
  useEffect(() => { urlRef.current = url; }, [url]);
  useEffect(() => { savedUrlRef.current = savedUrl; }, [savedUrl]);
  useEffect(() => { customIPsRef.current = customIPs; }, [customIPs]);
  useEffect(() => { savedCustomIPsRef.current = savedCustomIPs; }, [savedCustomIPs]);

  useEffect(() => {
    if (!ddClient) {
      return;
    }
    isMountedRef.current = true;

    // Load saved settings from backend
    const loadSettings = async (showSkeleton = true) => {
      if (showSkeleton) setIsLoadingSettings(true);
      try {
        const result = await withTimeout(service.getSettings(), BACKEND_REQUEST_TIMEOUT_MS);
        if (isSettingsResponse(result)) {
          const settings = result;
          const url = settings.url || '';
          const ips = settings.customIPs || [];

          if (isMountedRef.current) {
            setUrl(url);
            setSavedUrl(url);
            setCustomIPs(ips);
            setSavedCustomIPs(ips);
          }
        } else if (isMountedRef.current) {
          showSnackbar('Unexpected settings response format', 'error');
        }
      } catch (error) {
        // Silently fall back to localStorage if the backend is unavailable.
        const savedUrl = localStorage.getItem('url') || '';
        const savedIps = JSON.parse(localStorage.getItem('customIPs') || '[]');
        if (isMountedRef.current) {
          setUrl(savedUrl);
          setSavedUrl(savedUrl);
          setCustomIPs(savedIps);
          setSavedCustomIPs(savedIps);
        }
      } finally {
        if (showSkeleton && isMountedRef.current) {
          setIsLoadingSettings(false);
        }
      }
    };

    loadSettings();

    // Poll for external settings changes, but skip if the user has unsaved edits
    const pollInterval = setInterval(() => {
      const urlClean = urlRef.current === savedUrlRef.current;
      const ipsClean = JSON.stringify(customIPsRef.current) === JSON.stringify(savedCustomIPsRef.current);
      if (isMountedRef.current && urlClean && ipsClean) {
        loadSettings(false);
      }
    }, 5000);

    return () => {
      isMountedRef.current = false;
      clearInterval(pollInterval);
    };
  }, [ddClient, service, showSnackbar]);

  const hasUnsavedChanges = () => {
    if (url !== savedUrl) return true;
    if (JSON.stringify(customIPs.slice().sort()) !== JSON.stringify(savedCustomIPs.slice().sort())) return true;
    return false;
  };

  const handleAddIP = () => {
    const trimmed = newIP.trim();
    if (!trimmed) return;

    // Basic IP validation: try to match IPv4 or IPv6 patterns
    const ipv4Re = /^(\d{1,3}\.){3}\d{1,3}$/;
    const ipv6Re = /^[0-9a-fA-F:]+$/;
    if (!ipv4Re.test(trimmed) && !ipv6Re.test(trimmed)) {
      setIpError('Enter a valid IPv4 or IPv6 address');
      return;
    }

    if (customIPs.includes(trimmed)) {
      setIpError('Address already added');
      return;
    }

    setCustomIPs((prev) => [...prev, trimmed]);
    setNewIP('');
    setIpError('');
    setHasBeenSaved(false);
  };

  const handleSave = async () => {
    // Reset errors
    setUrlError('');

    if (!ddClient) {
      showSnackbar('Docker Desktop client unavailable', 'error');
      return;
    }

    // Validate URL
    if (!url) {
      setUrlError('URL is required');
      return;
    }
    if (!/^https?:\/\/[^/\\]/.test(url)) {
      setUrlError('Enter a valid URL (e.g. http://localhost:8080)');
      return;
    }

    setIsSaving(true);

    try {
      const settings = { url, customIPs };

      await withTimeout(
        ddClient.extension.vm?.service?.post('/settings', settings) ?? Promise.resolve(),
        BACKEND_REQUEST_TIMEOUT_MS,
      );

      // Save settings locally after successful backend save
      localStorage.setItem('url', url);
      localStorage.setItem('customIPs', JSON.stringify(customIPs));

      // Update saved state to disable button
      setSavedUrl(url);
      setSavedCustomIPs([...customIPs]);
      setHasBeenSaved(true);
      showSnackbar('Settings saved', 'success');

      // Disable save button to prevent rapid re-submission
      setIsDebouncing(true);
      const debounceTimer = setTimeout(() => {
        if (isMountedRef.current) {
          setIsDebouncing(false);
        }
      }, SAVE_DEBOUNCE_MS);

      return () => clearTimeout(debounceTimer);
    } catch (error) {
      // On error, don't update saved state so button stays enabled
      console.error('Failed to save settings:', error);
      showSnackbar('Failed to save settings', 'error');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Stack spacing={1}>
      <Typography variant="subtitle1">Settings</Typography>
      {proxyUnreachable && (
        <Alert
          severity="warning"
          sx={{ alignItems: 'center' }}
          action={
            onProxyHelp && (
              <Button color="inherit" size="small" onClick={onProxyHelp}>
                Get help
              </Button>
            )
          }
        >
          Extension backend not responding. Your last saved settings are shown below, but changes cannot be saved.
        </Alert>
      )}
      {isLoadingSettings ? (
        <Skeleton variant="rectangular" height={40} />
      ) : (
        <>
          <TextField
            type="text"
            label="IMDS server URL"
            placeholder="http://localhost:8080"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              setUrlError('');
              setHasBeenSaved(false);
            }}
            variant="outlined"
            size="small"
            error={!!urlError}
            helperText={urlError || 'Examples: http://localhost:8080, https://api.example.com'}
            fullWidth
            spellCheck={false}
          />

          <Typography variant="subtitle2" sx={{ mt: 2 }}>IMDS Addresses</Typography>
          <Typography variant="caption" color="text.secondary">
            IP addresses to intercept (IPv4 or IPv6).
          </Typography>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mb: 1 }}>
            {customIPs.map((ip) => (
              <Chip
                key={ip}
                label={ip}
                size="small"
                onDelete={() => {
                  setCustomIPs((prev) => prev.filter((i) => i !== ip));
                  setHasBeenSaved(false);
                }}
              />
            ))}
          </Box>
          <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
            <TextField
              type="text"
              label="IP address"
              placeholder="e.g. 10.0.0.1 or fd00::1"
              value={newIP}
              onChange={(e) => { setNewIP(e.target.value); setIpError(''); }}
              variant="outlined"
              size="small"
              error={!!ipError}
              helperText={ipError}
              spellCheck={false}
              sx={{ flex: 1 }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleAddIP();
                }
              }}
            />
            <IconButton onClick={handleAddIP} size="small" sx={{ mt: 0.5 }}>
              <AddIcon />
            </IconButton>
          </Box>

          <Button
            variant="contained"
            onClick={handleSave}
            disabled={isSaving || !hasUnsavedChanges() || isDebouncing || proxyUnreachable}
            sx={{ alignSelf: 'flex-start', mt: 1 }}
          >
            {isSaving ? 'Saving...' : hasBeenSaved ? 'Saved' : 'Save Settings'}
          </Button>
        </>
      )}
    </Stack>
  );
}
