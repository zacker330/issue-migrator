import React from 'react';
import { Alert, Table, Badge } from 'react-bootstrap';
import type { MigrationResult } from '../types';

interface MigrationProgressProps {
  result: MigrationResult;
}

const MigrationProgress: React.FC<MigrationProgressProps> = ({ result }) => {
  const successCount = result.success.length;
  const failedCount = result.failed.length;
  const totalCount = successCount + failedCount;

  return (
    <>
      <Alert variant={failedCount === 0 ? 'success' : failedCount === totalCount ? 'danger' : 'warning'}>
        <Alert.Heading>Migration Complete</Alert.Heading>
        <p>
          Successfully migrated {successCount} of {totalCount} issues.
          {failedCount > 0 && ` ${failedCount} issues failed to migrate.`}
        </p>
      </Alert>

      {result.success.length > 0 && (
        <>
          <h5 className="mt-4">Successfully Migrated Issues</h5>
          <Table striped bordered hover>
            <thead>
              <tr>
                <th>Original ID</th>
                <th>New ID</th>
                <th>New URL</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {result.success.map((status, idx) => (
                <tr key={idx}>
                  <td>#{status.original_id}</td>
                  <td>#{status.new_id}</td>
                  <td>
                    <a href={status.new_url} target="_blank" rel="noopener noreferrer">
                      {status.new_url}
                    </a>
                  </td>
                  <td>
                    <Badge bg="success">Success</Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </>
      )}

      {result.failed.length > 0 && (
        <>
          <h5 className="mt-4">Failed Migrations</h5>
          <Table striped bordered hover>
            <thead>
              <tr>
                <th>Original ID</th>
                <th>Error</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {result.failed.map((status, idx) => (
                <tr key={idx}>
                  <td>#{status.original_id}</td>
                  <td>{status.error}</td>
                  <td>
                    <Badge bg="danger">Failed</Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </>
      )}
    </>
  );
};

export default MigrationProgress;