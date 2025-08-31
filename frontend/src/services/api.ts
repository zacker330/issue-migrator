import axios from 'axios';
import type { Issue, MigrationConfig, MigrationResult, MigrateRequest } from '../types';

const API_BASE_URL = 'http://localhost:8080/api';

export const fetchGitHubIssues = async (config: MigrationConfig): Promise<Issue[]> => {
  const response = await axios.post(`${API_BASE_URL}/github/issues`, {
    owner: config.owner,
    repo: config.repo,
    token: config.token,
  });
  return response.data.issues;
};

export const fetchGitLabIssues = async (config: MigrationConfig): Promise<Issue[]> => {
  const response = await axios.post(`${API_BASE_URL}/gitlab/issues`, {
    base_url: config.baseUrl,
    project_id: config.projectId,
    token: config.token,
  });
  return response.data.issues;
};

export const migrateIssues = async (request: MigrateRequest): Promise<MigrationResult> => {
  const payload = {
    direction: request.direction,
    source: {
      type: request.source.type,
      owner: request.source.owner,
      repo: request.source.repo,
      project_id: request.source.projectId,
      base_url: request.source.baseUrl,
      token: request.source.token,
      session: request.source.session || '',
    },
    target: {
      type: request.target.type,
      owner: request.target.owner,
      repo: request.target.repo,
      project_id: request.target.projectId,
      base_url: request.target.baseUrl,
      token: request.target.token,
      session: request.target.session || '',
    },
    issue_ids: request.issueIds,
  };

  // Use main endpoint with image handling
  const response = await axios.post(`${API_BASE_URL}/migrate`, payload);
  return response.data;
};