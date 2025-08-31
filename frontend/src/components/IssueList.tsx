import React, { useState, useMemo } from 'react';
import { Table, Form, Button, Badge, Pagination } from 'react-bootstrap';
import type { Issue } from '../types';

interface IssueListProps {
  issues: Issue[];
  selectedIssues: number[];
  onSelectionChange: (selected: number[]) => void;
  onMigrate: () => void;
  loading: boolean;
}

const ITEMS_PER_PAGE = 10;

const IssueList: React.FC<IssueListProps> = ({
  issues,
  selectedIssues,
  onSelectionChange,
  onMigrate,
  loading,
}) => {
  const [currentPage, setCurrentPage] = useState(1);
  
  const totalPages = Math.ceil(issues.length / ITEMS_PER_PAGE);
  
  const paginatedIssues = useMemo(() => {
    const startIndex = (currentPage - 1) * ITEMS_PER_PAGE;
    const endIndex = startIndex + ITEMS_PER_PAGE;
    return issues.slice(startIndex, endIndex);
  }, [issues, currentPage]);

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      // Select all issues on the current page
      const currentPageIds = paginatedIssues.map((issue) => issue.id);
      const otherSelectedIds = selectedIssues.filter(
        id => !paginatedIssues.some(issue => issue.id === id)
      );
      onSelectionChange([...otherSelectedIds, ...currentPageIds]);
    } else {
      // Deselect all issues on the current page
      const currentPageIds = paginatedIssues.map((issue) => issue.id);
      onSelectionChange(selectedIssues.filter(id => !currentPageIds.includes(id)));
    }
  };

  const handleSelectAllGlobal = (checked: boolean) => {
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

  const isAllCurrentPageSelected = paginatedIssues.length > 0 && 
    paginatedIssues.every(issue => selectedIssues.includes(issue.id));

  const renderPagination = () => {
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
        <Pagination.Prev key="prev" onClick={() => setCurrentPage(Math.max(1, currentPage - 1))} disabled={currentPage === 1} />
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
        <Pagination.Next key="next" onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))} disabled={currentPage === totalPages} />,
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
      <div className="d-flex justify-content-between align-items-center mb-3">
        <div>
          <h4>Found {issues.length} issues</h4>
          <p className="text-muted mb-0">
            Showing {Math.min((currentPage - 1) * ITEMS_PER_PAGE + 1, issues.length)} - {Math.min(currentPage * ITEMS_PER_PAGE, issues.length)} of {issues.length} issues
            {selectedIssues.length > 0 && ` • ${selectedIssues.length} selected`}
          </p>
        </div>
        <div className="d-flex gap-2">
          {issues.length > ITEMS_PER_PAGE && (
            <Button
              variant="outline-primary"
              size="sm"
              onClick={() => handleSelectAllGlobal(selectedIssues.length !== issues.length)}
            >
              {selectedIssues.length === issues.length ? 'Deselect All' : 'Select All Issues'}
            </Button>
          )}
          <Button
            variant="success"
            onClick={onMigrate}
            disabled={loading || selectedIssues.length === 0}
          >
            {loading ? 'Migrating...' : `Migrate ${selectedIssues.length} Selected Issues`}
          </Button>
        </div>
      </div>

      <Table striped bordered hover responsive>
        <thead>
          <tr>
            <th style={{ width: '50px' }}>
              <Form.Check
                type="checkbox"
                checked={isAllCurrentPageSelected}
                onChange={(e) => handleSelectAll(e.target.checked)}
                title="Select/Deselect all on this page"
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
          {paginatedIssues.map((issue) => (
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

      {renderPagination()}
    </>
  );
};

export default IssueList;