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

/**
 * Connectivity status for a single configured IMDS IP address
 */
export interface AddressStatus {
  ip: string;
  connected: boolean;
}

/**
 * Container information from Docker Desktop
 */
export interface ContainerInfo {
  containerId: string;
  name: string;
  labels: Record<string, string>;
  addresses: AddressStatus[];
}

/**
 * Settings response from backend
 */
export interface SettingsResponse {
  url?: string;
  customIPs?: string[];
}

/**
 * State of the IMDS proxy container
 */
export type ProxyContainerState = 'running' | 'paused' | 'stopped' | 'failed' | 'missing';

/**
 * Response from GET /containers
 */
export interface ContainersResponse {
  containers: ContainerInfo[];
  proxyStatus: ProxyContainerState;
}

/**
 * Type guard to validate settings response
 */
export const isSettingsResponse = (value: unknown): value is SettingsResponse => {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
};

/**
 * Type guard to validate containers response
 */
export const isContainersResponse = (value: unknown): value is ContainersResponse => {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }
  const v = value as Record<string, unknown>;
  return Array.isArray(v.containers) && typeof v.proxyStatus === 'string';
};
