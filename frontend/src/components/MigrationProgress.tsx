import React, { useState, useMemo } from 'react';
import { Alert, Table, Badge, Pagination, ButtonGroup, Button } from 'react-bootstrap';
import type { MigrationResult } from '../types';

interface MigrationProgressProps {
  result: MigrationResult;
}

const ITEMS_PER_PAGE = 10;

const MigrationProgress: React.FC<MigrationProgressProps> = ({ result }) => {
  const [currentSuccessPage, setCurrentSuccessPage] = useState(1);
  const [currentFailedPage, setCurrentFailedPage] = useState(1);
  const [viewMode, setViewMode] = useState<'all' | 'success' | 'failed'>('all');

  const successCount = result.success.length;
  const failedCount = result.failed.length;
  const totalCount = successCount + failedCount;

  const totalSuccessPages = Math.ceil(successCount / ITEMS_PER_PAGE);
  const totalFailedPages = Math.ceil(failedCount / ITEMS_PER_PAGE);

  const paginatedSuccess = useMemo(() => {
    const startIndex = (currentSuccessPage - 1) * ITEMS_PER_PAGE;
    const endIndex = startIndex + ITEMS_PER_PAGE;
    return result.success.slice(startIndex, endIndex);
  }, [result.success, currentSuccessPage]);

  const paginatedFailed = useMemo(() => {
    const startIndex = (currentFailedPage - 1) * ITEMS_PER_PAGE;
    const endIndex = startIndex + ITEMS_PER_PAGE;
    return result.failed.slice(startIndex, endIndex);
  }, [result.failed, currentFailedPage]);

  const renderPagination = (
    currentPage: number,
    totalPages: number,
    setCurrentPage: (page: number) => void
  ) => {
    if (totalPages <= 1) return null;

    const items = [];
    const maxVisible = 5;
    let startPage = Math.max(1, currentPage - Math.floor(maxVisible / 2));
    let endPage = Math.min(totalPages, startPage + maxVisible - 1);

    if (endPage - startPage < maxVisible - 1) {
      startPage = Math.max(1, endPage - maxVisible + 1);
    }

    // First page
    if (startPage > 1) {
      items.push(
        <Pagination.First key="first" onClick={() => setCurrentPage(1)} />,
        <Pagination.Prev 
          key="prev" 
          onClick={() => setCurrentPage(Math.max(1, currentPage - 1))} 
          disabled={currentPage === 1} 
        />
      );
    }

    // Page numbers
    for (let i = startPage; i <= endPage; i++) {
      items.push(
        <Pagination.Item
          key={i}
          active={i === currentPage}
          onClick={() => setCurrentPage(i)}
        >
          {i}
        </Pagination.Item>
      );
    }

    // Last page
    if (endPage < totalPages) {
      items.push(
        <Pagination.Next 
          key="next" 
          onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))} 
          disabled={currentPage === totalPages} 
        />,
        <Pagination.Last key="last" onClick={() => setCurrentPage(totalPages)} />
      );
    }

    return (
      <div className="d-flex justify-content-center mt-3">
        <Pagination>{items}</Pagination>
      </div>
    );
  };

  return (
    <>
      <Alert variant={failedCount === 0 ? 'success' : failedCount === totalCount ? 'danger' : 'warning'}>
        <Alert.Heading>Migration Complete</Alert.Heading>
        <p className="mb-3">
          Successfully migrated <strong>{successCount}</strong> of <strong>{totalCount}</strong> issues.
          {failedCount > 0 && (
            <>
              <br />
              <strong>{failedCount}</strong> issues failed to migrate.
            </>
          )}
        </p>
        
        <ButtonGroup size="sm">
          <Button 
            variant={viewMode === 'all' ? 'primary' : 'outline-primary'}
            onClick={() => setViewMode('all')}
          >
            Show All
          </Button>
          <Button 
            variant={viewMode === 'success' ? 'success' : 'outline-success'}
            onClick={() => setViewMode('success')}
            disabled={successCount === 0}
          >
            Success Only ({successCount})
          </Button>
          <Button 
            variant={viewMode === 'failed' ? 'danger' : 'outline-danger'}
            onClick={() => setViewMode('failed')}
            disabled={failedCount === 0}
          >
            Failed Only ({failedCount})
          </Button>
        </ButtonGroup>
      </Alert>

      {(viewMode === 'all' || viewMode === 'success') && result.success.length > 0 && (
        <>
          <h5 className="mt-4">
            Successfully Migrated Issues ({successCount})
            {successCount > ITEMS_PER_PAGE && (
              <small className="text-muted ms-2">
                Showing {Math.min((currentSuccessPage - 1) * ITEMS_PER_PAGE + 1, successCount)} - {Math.min(currentSuccessPage * ITEMS_PER_PAGE, successCount)}
              </small>
            )}
          </h5>
          <Table striped bordered hover responsive>
            <thead>
              <tr>
                <th style={{ width: '120px' }}>Original ID</th>
                <th style={{ width: '120px' }}>New ID</th>
                <th>New URL</th>
                <th style={{ width: '100px' }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {paginatedSuccess.map((status, idx) => (
                <tr key={`success-${idx}`}>
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
          {renderPagination(currentSuccessPage, totalSuccessPages, setCurrentSuccessPage)}
        </>
      )}

      {(viewMode === 'all' || viewMode === 'failed') && result.failed.length > 0 && (
        <>
          <h5 className="mt-4">
            Failed Migrations ({failedCount})
            {failedCount > ITEMS_PER_PAGE && (
              <small className="text-muted ms-2">
                Showing {Math.min((currentFailedPage - 1) * ITEMS_PER_PAGE + 1, failedCount)} - {Math.min(currentFailedPage * ITEMS_PER_PAGE, failedCount)}
              </small>
            )}
          </h5>
          <Table striped bordered hover responsive>
            <thead>
              <tr>
                <th style={{ width: '120px' }}>Original ID</th>
                <th>Error</th>
                <th style={{ width: '100px' }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {paginatedFailed.map((status, idx) => (
                <tr key={`failed-${idx}`}>
                  <td>#{status.original_id}</td>
                  <td>
                    <div className="text-danger" style={{ maxWidth: '500px', wordBreak: 'break-word' }}>
                      {status.error}
                    </div>
                  </td>
                  <td>
                    <Badge bg="danger">Failed</Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          {renderPagination(currentFailedPage, totalFailedPages, setCurrentFailedPage)}
        </>
      )}
    </>
  );
};

export default MigrationProgress;