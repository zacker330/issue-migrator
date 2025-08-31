import React from 'react';
import { Form, Button } from 'react-bootstrap';
import type { MigrationConfig } from '../types';

interface SourceConfigProps {
  config: MigrationConfig;
  onChange: (config: MigrationConfig) => void;
  onFetch?: () => void;
  loading: boolean;
  isTarget?: boolean;
}

const SourceConfig: React.FC<SourceConfigProps> = ({
  config,
  onChange,
  onFetch,
  loading,
  isTarget = false,
}) => {
  const handleChange = (field: keyof MigrationConfig, value: any) => {
    onChange({ ...config, [field]: value });
  };

  return (
    <Form>
      <Form.Group className="mb-3">
        <Form.Label>Platform</Form.Label>
        <Form.Select
          value={config.type}
          onChange={(e) => handleChange('type', e.target.value)}
        >
          <option value="github">GitHub</option>
          <option value="gitlab">GitLab</option>
        </Form.Select>
      </Form.Group>

      {config.type === 'github' ? (
        <>
          <Form.Group className="mb-3">
            <Form.Label>Owner/Organization</Form.Label>
            <Form.Control
              type="text"
              placeholder="e.g., octocat"
              value={config.owner}
              onChange={(e) => handleChange('owner', e.target.value)}
            />
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Repository</Form.Label>
            <Form.Control
              type="text"
              placeholder="e.g., hello-world"
              value={config.repo}
              onChange={(e) => handleChange('repo', e.target.value)}
            />
          </Form.Group>
        </>
      ) : (
        <>
          <Form.Group className="mb-3">
            <Form.Label>GitLab URL</Form.Label>
            <Form.Control
              type="text"
              placeholder="https://gitlab.com"
              value={config.baseUrl}
              onChange={(e) => handleChange('baseUrl', e.target.value)}
            />
          </Form.Group>

          <Form.Group className="mb-3">
            <Form.Label>Project ID</Form.Label>
            <Form.Control
              type="number"
              placeholder="e.g., 123456"
              value={config.projectId || ''}
              onChange={(e) => handleChange('projectId', parseInt(e.target.value) || 0)}
            />
          </Form.Group>
        </>
      )}

      <Form.Group className="mb-3">
        <Form.Label>Access Token</Form.Label>
        <Form.Control
          type="password"
          placeholder="Personal access token"
          value={config.token}
          onChange={(e) => handleChange('token', e.target.value)}
        />
        <Form.Text className="text-muted">
          {config.type === 'github' 
            ? 'GitHub personal access token with repo scope'
            : 'GitLab personal access token with api scope'}
        </Form.Text>
      </Form.Group>

      {config.type === 'github' && (
        <Form.Group className="mb-3">
          <Form.Label>Session Cookie (Optional)</Form.Label>
          <Form.Control
            type="password"
            placeholder="user_session cookie value"
            value={config.session || ''}
            onChange={(e) => handleChange('session', e.target.value)}
          />
          <Form.Text className="text-muted">
            GitHub session cookie for file uploads (found in browser DevTools → Application → Cookies → user_session)
          </Form.Text>
        </Form.Group>
      )}

      {!isTarget && (
        <Button
          variant="primary"
          onClick={onFetch}
          disabled={loading || !config.token || (config.type === 'github' ? !config.owner || !config.repo : !config.projectId)}
        >
          {loading ? 'Loading...' : 'Fetch Issues'}
        </Button>
      )}
    </Form>
  );
};

export default SourceConfig;