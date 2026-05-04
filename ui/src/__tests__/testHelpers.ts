/**
 * Copyright 2026 Matt Miller
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { vi } from 'vitest';
import { DockerDesktopServiceClient } from '../services/dockerDesktopService';
import { ContainerInfo } from '../types';

export const TEST_SETTINGS_URL = 'http://localhost:8080';
export const TEST_CONTAINER_ID = 'abc123';
const TEST_LABELS = { 'imds-proxy.enabled': 'true' };

/**
 * Creates a mock Docker Desktop client with configurable service methods
 */
export const createMockDockerDesktopClient = (overrides?: {
  get?: any;
  post?: any;
}) => ({
  extension: {
    vm: {
      service: {
        get: overrides?.get ?? vi.fn(),
        post: overrides?.post ?? vi.fn().mockResolvedValue(undefined),
      },
    },
  },
});

/**
 * Creates a mock DockerDesktopServiceClient with configurable overrides
 */
export const createMockService = (
  overrides?: Partial<DockerDesktopServiceClient>
): DockerDesktopServiceClient => ({
  getSettings: vi.fn().mockResolvedValue({ url: '' }),
  getContainers: vi.fn().mockResolvedValue([]),
  getComposeProjectName: vi.fn().mockResolvedValue({ projectName: 'barnacle', configFiles: '/path/to/docker-compose.yaml' }),
  ...overrides,
});

/**
 * Creates mock container data for testing
 */
export const createMockContainer = (overrides?: Partial<ContainerInfo>): ContainerInfo => ({
  containerId: 'abc123def456ghi789jkl012mno345pqr678',
  name: '/test-container',
  labels: { ...TEST_LABELS },
  addresses: [
    { ip: '169.254.169.254', connected: true },
    { ip: 'fd00:ec2::254', connected: true },
  ],
  ...overrides,
});

/**
 * Creates an array of mock containers for testing table/list functionality
 */
export const createMockContainers = (count: number = 3): ContainerInfo[] => {
  return Array.from({ length: count }, (_, i) => ({
    containerId: `container-id-${i.toString().padStart(32, '0')}`,
    name: `/container-${i}`,
    labels: { ...TEST_LABELS },
    addresses: [
      { ip: '169.254.169.254', connected: true },
      { ip: 'fd00:ec2::254', connected: true },
    ],
  }));
};

interface MockGetResponses {
  settings?: any;
  containers?: any;
}

/**
 * Creates a mock implementation for service.get() with configurable responses
 * for /settings and /containers paths. Use this to simplify repetitive mock setup.
 *
 * @example
 * // Simple success case
 * mockGet.mockImplementation(createMockGetImplementation({
 *   settings: { url: 'http://example.com' },
 *   containers: []
 * }));
 *
 * @example
 * // Error case
 * mockGet.mockImplementation(createMockGetImplementation({
 *   containers: new Error('network error')
 * }));
 *
 * @example
 * // Never-resolving promise
 * mockGet.mockImplementation(createMockGetImplementation({
 *   containers: new Promise(() => {})
 * }));
 */
export const createMockGetImplementation = (responses: MockGetResponses) => {
  return (path: string) => {
    if (path === '/settings') {
      if ('settings' in responses) {
        const setting = responses.settings;
        if (setting instanceof Error) {
          return Promise.reject(setting);
        }
        if (setting instanceof Promise) {
          return setting;
        }
        return Promise.resolve(setting);
      }
      return Promise.resolve({ url: '' });
    }
    if (path === '/containers') {
      if ('containers' in responses) {
        const containers = responses.containers;
        // Allow rejection by passing an Error
        if (containers instanceof Error) {
          return Promise.reject(containers);
        }
        // Allow custom promises (e.g., never resolving)
        if (containers instanceof Promise) {
          return containers;
        }
        return Promise.resolve(containers);
      }
      return Promise.resolve({ containers: [], proxyStatus: 'running' });
    }
    return Promise.reject(new Error('unknown path'));
  };
};
