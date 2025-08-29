import React from 'react';
import { Table, Form, Button, Badge } from 'react-bootstrap';
import type { Issue } from '../types';

interface IssueListProps {
  issues: Issue[];
  selectedIssues: number[];
  onSelectionChange: (selected: number[]) => void;
  onMigrate: () => void;
  loading: boolean;
}

const IssueList: React.FC<IssueListProps> = ({
  issues,
  selectedIssues,
  onSelectionChange,
  onMigrate,
  loading,
}) => {
  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      onSelectionChange(issues.map((issue) => issue.id));
    } else {
      onSelectionChange([]);
    }
  };

  const handleSelectIssue = (id: number, checked: boolean) => {
    if (checked) {
      onSelectionChange([...selectedIssues, id]);
    } else {
      onSelectionChange(selectedIssues.filter((issueId) => issueId !== id));
    }
  };

  const getStateBadge = (state: string) => {
    const variant = state.toLowerCase() === 'open' || state.toLowerCase() === 'opened' ? 'success' : 'secondary';
    return <Badge bg={variant}>{state}</Badge>;
  };

  return (
    <>
      <div className="d-flex justify-content-between align-items-center mb-3">
        <h4>Found {issues.length} issues</h4>
        <Button
          variant="success"
          onClick={onMigrate}
          disabled={loading || selectedIssues.length === 0}
        >
          {loading ? 'Migrating...' : `Migrate ${selectedIssues.length} Selected Issues`}
        </Button>
      </div>

      <Table striped bordered hover responsive>
        <thead>
          <tr>
            <th style={{ width: '50px' }}>
              <Form.Check
                type="checkbox"
                checked={selectedIssues.length === issues.length && issues.length > 0}
                onChange={(e) => handleSelectAll(e.target.checked)}
              />
            </th>
            <th>#</th>
            <th>Title</th>
            <th>State</th>
            <th>Author</th>
            <th>Labels</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {issues.map((issue) => (
            <tr key={issue.id}>
              <td>
                <Form.Check
                  type="checkbox"
                  checked={selectedIssues.includes(issue.id)}
                  onChange={(e) => handleSelectIssue(issue.id, e.target.checked)}
                />
              </td>
              <td>{issue.id}</td>
              <td>
                <a href={issue.url} target="_blank" rel="noopener noreferrer">
                  {issue.title}
                </a>
              </td>
              <td>{getStateBadge(issue.state)}</td>
              <td>{issue.author}</td>
              <td>
                {issue.labels.map((label, idx) => (
                  <Badge key={idx} bg="info" className="me-1">
                    {label}
                  </Badge>
                ))}
              </td>
              <td>{new Date(issue.created_at).toLocaleDateString()}</td>
            </tr>
          ))}
        </tbody>
      </Table>
    </>
  );
};

export default IssueList;