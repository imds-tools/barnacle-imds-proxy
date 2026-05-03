// Copyright 2026 Matt Miller
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import React from 'react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SettingsForm } from '../components/SettingsForm';
import { SAVE_DEBOUNCE_MS } from '../constants';
import { createMockDockerDesktopClient, createMockService } from './testHelpers';

const SETTINGS_URL = 'http://example.com';
const LOCAL_URL = 'http://local';
const NEW_URL = 'http://new';
const OTHER_URL = 'http://other';
const NEW_URL_VALUE = 'http://newurl';
const TEST_URL = 'http://test.com';

describe('SettingsForm', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads settings from the backend', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    const showSnackbar = vi.fn();

    render(
      <SettingsForm
        ddClient={createMockDockerDesktopClient() as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    await waitFor(() => {
      const input = screen.getByLabelText(/IMDS server URL/i) as HTMLInputElement;
      expect(input.value).toBe(SETTINGS_URL);
    });

    expect(showSnackbar).not.toHaveBeenCalled();
  });

  it('falls back to local storage on load errors', async () => {
    localStorage.setItem('url', LOCAL_URL);
    const service = createMockService({
      getSettings: vi.fn().mockRejectedValue(new Error('boom')),
    });
    const showSnackbar = vi.fn();

    render(
      <SettingsForm
        ddClient={createMockDockerDesktopClient() as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    await waitFor(() => {
      const input = screen.getByLabelText(/IMDS server URL/i) as HTMLInputElement;
      expect(input.value).toBe(LOCAL_URL);
    });

    expect(showSnackbar).not.toHaveBeenCalled();
  });

  it('validates required URL before saving', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    render(
      <SettingsForm
        ddClient={client as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    const input = await screen.findByLabelText(/IMDS server URL/i);
    fireEvent.change(input, { target: { value: '' } });

    fireEvent.click(screen.getByRole('button', { name: /save settings/i }));

    await waitFor(() => {
      expect(screen.getByText('URL is required')).toBeTruthy();
    });
    expect(client.extension.vm.service.post).not.toHaveBeenCalled();
  });

  it('validates URL format before saving', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    render(
      <SettingsForm
        ddClient={client as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    const input = await screen.findByLabelText(/IMDS server URL/i);
    fireEvent.change(input, { target: { value: 'not a url' } });

    fireEvent.click(screen.getByRole('button', { name: /save settings/i }));

    await waitFor(() => {
      expect(screen.getByText('Enter a valid URL (e.g. http://localhost:8080)')).toBeTruthy();
    });
    expect(client.extension.vm.service.post).not.toHaveBeenCalled();
  });

  it('rejects malformed URLs', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    render(
      <SettingsForm
        ddClient={client as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    const input = await screen.findByLabelText(/IMDS server URL/i);
    const invalidUrls = ['not a url', 'ftp://example.com', 'http://', 'http:///\\localhost:3000'];

    for (const invalidUrl of invalidUrls) {
      fireEvent.change(input, { target: { value: invalidUrl } });
      fireEvent.click(screen.getByRole('button', { name: /save settings/i }));
      await waitFor(() => {
        expect(screen.getByText('Enter a valid URL (e.g. http://localhost:8080)')).toBeTruthy();
      });
    }
    expect(client.extension.vm.service.post).not.toHaveBeenCalled();
  });

  it('saves settings and debounces the button', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: '' }),
    });
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    render(
      <SettingsForm
        ddClient={client as any}
        service={service}
        showSnackbar={showSnackbar}
      />
    );

    const input = await screen.findByLabelText(/IMDS server URL/i);

    vi.useFakeTimers();
    fireEvent.change(input, { target: { value: 'http://new' } });

    const button = screen.getByRole('button', { name: /save settings/i });
    fireEvent.click(button);

    // Wait for the save operation to complete
    await act(async () => {
      await Promise.resolve(); // Flush microtasks
    });

    expect(client.extension.vm.service.post).toHaveBeenCalledWith('/settings', {
      url: NEW_URL,
      customIPs: expect.any(Array),
    });
    expect(localStorage.getItem('url')).toBe(NEW_URL);
    expect(showSnackbar).toHaveBeenCalledWith('Settings saved', 'success');
    expect((button as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(input, { target: { value: OTHER_URL } });
    expect((button as HTMLButtonElement).disabled).toBe(true);

    act(() => {
      vi.advanceTimersByTime(SAVE_DEBOUNCE_MS);
    });
    expect((button as HTMLButtonElement).disabled).toBe(false);

    vi.useRealTimers();
  });

  it('displays loading skeleton while loading settings', async () => {
    const service = createMockService();
    const showSnackbar = vi.fn();

    const { container } = render(
      <SettingsForm ddClient={createMockDockerDesktopClient() as any} service={service} showSnackbar={showSnackbar} />
    );

    // Should show skeleton initially
    const skeleton = container.querySelector('.MuiSkeleton-root');
    expect(skeleton).not.toBeNull();

    // Wait for settings to load
    await waitFor(() => {
      expect(screen.getByLabelText(/IMDS server URL/i)).toBeDefined();
    });
  });

  it('handles save error gracefully', async () => {
    const service = createMockService();
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    // Mock a failed save
    client.extension.vm.service.post.mockRejectedValueOnce(new Error('Network error'));

    render(<SettingsForm ddClient={client as any} service={service} showSnackbar={showSnackbar} />);

    await waitFor(() => expect(screen.getByLabelText(/IMDS server URL/i)).toBeDefined());

    const input = screen.getByLabelText(/IMDS server URL/i);
    fireEvent.change(input, { target: { value: NEW_URL_VALUE } });

    const button = screen.getByRole('button', { name: /save settings/i });
    fireEvent.click(button);

    await waitFor(() => {
      expect(showSnackbar).toHaveBeenCalledWith('Failed to save settings', 'error');
    });

    //Button should be enabled after error
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it('handles missing Docker Desktop client', async () => {
    const service = createMockService();
    const showSnackbar = vi.fn();

    render(<SettingsForm ddClient={null} service={service} showSnackbar={showSnackbar} />);

    // Component renders, but should not call service methods without client
    expect(screen.getByLabelText(/IMDS server URL/i)).toBeDefined();
    expect(service.getSettings).not.toHaveBeenCalled();

    // Attempting to save should show error
    const button = screen.getByRole('button', { name: /save settings/i });
    const input = screen.getByLabelText(/IMDS server URL/i);
    fireEvent.change(input, { target: { value: TEST_URL } });
    fireEvent.click(button);

    await waitFor(() => {
      expect(showSnackbar).toHaveBeenCalledWith('Docker Desktop client unavailable', 'error');
    });
  });

  it('adds a valid IPv4 address to the list', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    render(<SettingsForm ddClient={createMockDockerDesktopClient() as any} service={service} showSnackbar={vi.fn()} />);

    await waitFor(() => expect(screen.getByLabelText(/IP address/i)).toBeDefined());

    fireEvent.change(screen.getByLabelText(/IP address/i), { target: { value: '169.254.169.254' } });
    fireEvent.click(screen.getByRole('button', { name: '' })); // AddIcon button

    await waitFor(() => expect(screen.getByText('169.254.169.254')).toBeTruthy());
  });

  it('rejects an invalid IP address', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL }),
    });
    render(<SettingsForm ddClient={createMockDockerDesktopClient() as any} service={service} showSnackbar={vi.fn()} />);

    await waitFor(() => expect(screen.getByLabelText(/IP address/i)).toBeDefined());

    fireEvent.change(screen.getByLabelText(/IP address/i), { target: { value: 'not-an-ip' } });
    fireEvent.click(screen.getByRole('button', { name: '' }));

    await waitFor(() => expect(screen.getByText('Enter a valid IPv4 or IPv6 address')).toBeTruthy());
  });

  it('rejects a duplicate IP address', async () => {
    const service = createMockService({
      getSettings: vi.fn().mockResolvedValue({ url: SETTINGS_URL, customIPs: ['169.254.169.254'] }),
    });
    render(<SettingsForm ddClient={createMockDockerDesktopClient() as any} service={service} showSnackbar={vi.fn()} />);

    await waitFor(() => expect(screen.getByLabelText(/IP address/i)).toBeDefined());

    fireEvent.change(screen.getByLabelText(/IP address/i), { target: { value: '169.254.169.254' } });
    fireEvent.click(screen.getByRole('button', { name: '' }));

    await waitFor(() => expect(screen.getByText('Address already added')).toBeTruthy());
  });

  it('handles unexpected settings response format', async () => {
    const mockService = createMockService({
      getSettings: vi.fn().mockResolvedValue(null), // null is not a valid settings response
    });
    const showSnackbar = vi.fn();
    const client = createMockDockerDesktopClient();

    render(<SettingsForm ddClient={client as any} service={mockService} showSnackbar={showSnackbar} />);

    await waitFor(() => {
      expect(showSnackbar).toHaveBeenCalledWith('Unexpected settings response format', 'error');
    });
  });
});
