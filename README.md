# Issue Migrator

A web application for migrating issues between GitHub and GitLab platforms.

## Features

- Fetch issues from GitHub repositories or GitLab projects
- Select specific issues to migrate
- Migrate issues with descriptions, labels, and comments
- **Image Migration**: Automatically downloads and re-uploads images when migrating to GitLab
- Support for both GitHub to GitLab and GitLab to GitHub migrations
- Track migration progress and results

## Architecture

- **Backend**: Go with Gin framework
- **Frontend**: React with TypeScript and Bootstrap
- **APIs**: GitHub API v3 and GitLab API v4

## Prerequisites

- Go 1.21 or higher
- Node.js 22 or higher
- GitHub personal access token (with `repo` scope)
- GitLab personal access token (with `api` scope)

## Setup

### Backend

1. Navigate to the backend directory:
   ```bash
   cd backend
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Create a `.env` file (optional):
   ```bash
   cp .env.example .env
   ```

4. Run the backend server:
   ```bash
   go run main.go
   ```

   The server will start on `http://localhost:8080`

### Frontend

1. Navigate to the frontend directory:
   ```bash
   cd frontend
   ```

2. Install dependencies:
   ```bash
   npm install
   ```

3. Start the development server:
   ```bash
   npm run dev
   ```

   The application will be available at `http://localhost:3000`

## Usage

1. **Configure Source Platform**: 
   - Select GitHub or GitLab as your source
   - For GitHub: Enter owner/organization and repository name
   - For GitLab: Enter GitLab URL and project ID
   - Provide your personal access token

2. **Configure Target Platform**:
   - Select the opposite platform as your target
   - Enter the required configuration details
   - Provide your personal access token for the target platform

3. **Fetch and Select Issues**:
   - Click "Fetch Issues" to retrieve all issues from the source
   - Select the issues you want to migrate using checkboxes
   - Review the selected issues

4. **Migrate**:
   - Click "Migrate Selected Issues"
   - Monitor the migration progress
   - Review successful and failed migrations

## API Endpoints

- `GET /api/health` - Health check endpoint
- `POST /api/github/issues` - Fetch issues from GitHub
- `POST /api/gitlab/issues` - Fetch issues from GitLab
- `POST /api/migrate` - Migrate issues between platforms

## Security Notes

- Never commit your access tokens to version control
- Use environment variables or secure credential storage
- Ensure CORS is properly configured for production use
- Implement rate limiting for production deployments

## Development

### Running Tests

Backend:
```bash
cd backend
go test ./...
```

Frontend:
```bash
cd frontend
npm test
```

### Building for Production

Backend:
```bash
cd backend
go build -o issue-migrator
```

Frontend:
```bash
cd frontend
npm run build
```

## License

MIT