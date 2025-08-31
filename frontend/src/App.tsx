import { useState } from 'react';
import 'bootstrap/dist/css/bootstrap.min.css';
import { Container, Tab, Tabs } from 'react-bootstrap';
import SourceConfig from './components/SourceConfig';
import IssueList from './components/IssueList';
import MigrationProgress from './components/MigrationProgress';
import type { Issue, MigrationConfig, MigrationResult } from './types';
import { fetchGitHubIssues, fetchGitLabIssues, migrateIssues } from './services/api';

function App() {
  const [sourceConfig, setSourceConfig] = useState<MigrationConfig>({
    type: 'github',
    owner: '',
    repo: '',
    token: '',
    session: '',
    baseUrl: '',
    projectId: 0,
  });

  const [targetConfig, setTargetConfig] = useState<MigrationConfig>({
    type: 'gitlab',
    owner: '',
    repo: '',
    token: '',
    session: '',
    baseUrl: 'https://gitlab.com',
    projectId: 0,
  });

  const [sourceIssues, setSourceIssues] = useState<Issue[]>([]);
  const [selectedIssues, setSelectedIssues] = useState<number[]>([]);
  const [migrationResult, setMigrationResult] = useState<MigrationResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('configure');

  const handleFetchIssues = async () => {
    setLoading(true);
    try {
      let issues: Issue[];
      if (sourceConfig.type === 'github') {
        issues = await fetchGitHubIssues(sourceConfig);
      } else {
        issues = await fetchGitLabIssues(sourceConfig);
      }
      setSourceIssues(issues);
      // Automatically switch to issues tab after successful fetch
      setActiveTab('issues');
    } catch (error) {
      console.error('Failed to fetch issues:', error);
      alert('Failed to fetch issues. Please check your configuration.');
    } finally {
      setLoading(false);
    }
  };

  const handleMigrate = async () => {
    if (selectedIssues.length === 0) {
      alert('Please select at least one issue to migrate');
      return;
    }

    setLoading(true);
    try {
      const result = await migrateIssues({
        direction: `${sourceConfig.type}-to-${targetConfig.type}`,
        source: sourceConfig,
        target: targetConfig,
        issueIds: selectedIssues,
      });
      setMigrationResult(result);
    } catch (error) {
      console.error('Migration failed:', error);
      alert('Migration failed. Please check your configuration.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Container className="py-5">
      <h1 className="mb-4">Issue Migrator</h1>
      <p className="lead mb-4">Migrate issues between GitHub and GitLab platforms</p>

      <Tabs activeKey={activeTab} onSelect={(k) => k && setActiveTab(k)} className="mb-4">
        <Tab eventKey="configure" title="Configure">
          <div className="row">
            <div className="col-md-6">
              <h3>Source Platform</h3>
              <SourceConfig
                config={sourceConfig}
                onChange={setSourceConfig}
                onFetch={handleFetchIssues}
                loading={loading}
              />
            </div>
            <div className="col-md-6">
              <h3>Target Platform</h3>
              <SourceConfig
                config={targetConfig}
                onChange={setTargetConfig}
                loading={loading}
                isTarget
              />
            </div>
          </div>
        </Tab>

        <Tab eventKey="issues" title="Select Issues" disabled={sourceIssues.length === 0}>
          <IssueList
            issues={sourceIssues}
            selectedIssues={selectedIssues}
            onSelectionChange={setSelectedIssues}
            onMigrate={handleMigrate}
            loading={loading}
          />
        </Tab>

        <Tab eventKey="results" title="Migration Results" disabled={!migrationResult}>
          {migrationResult && <MigrationProgress result={migrationResult} />}
        </Tab>
      </Tabs>
    </Container>
  );
}

export default App;