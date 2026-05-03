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
 * Per-provider IMDS proxying status for a container
 */
export interface ProviderStatus {
  name: string;
  ipv4Connected: boolean;
  ipv6Connected: boolean;
}

/**
 * Container information from Docker Desktop
 */
export interface ContainerInfo {
  containerId: string;
  name: string;
  labels: Record<string, string>;
  providers: ProviderStatus[];
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

/**
 * Returns true if the provider has full proxying (both IPv4 and IPv6)
 */
export const isProviderFullyConnected = (p: ProviderStatus): boolean =>
  p.ipv4Connected && p.ipv6Connected;

/**
 * Returns true if the provider has partial proxying (one of IPv4/IPv6)
 */
export const isProviderPartiallyConnected = (p: ProviderStatus): boolean =>
  p.ipv4Connected !== p.ipv6Connected;
