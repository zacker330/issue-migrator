export interface Issue {
  id: number;
  title: string;
  description: string;
  state: string;
  labels: string[];
  author: string;
  created_at: string;
  updated_at: string;
  url: string;
}

export interface MigrationConfig {
  type: 'github' | 'gitlab';
  owner: string;
  repo: string;
  token: string;
  baseUrl: string;
  projectId: number;
}

export interface MigrationStatus {
  original_id: number;
  new_id?: number;
  new_url?: string;
  error?: string;
}

export interface MigrationResult {
  success: MigrationStatus[];
  failed: MigrationStatus[];
}

export interface MigrateRequest {
  direction: string;
  source: MigrationConfig;
  target: MigrationConfig;
  issueIds: number[];
}