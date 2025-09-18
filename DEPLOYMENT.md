# Issue Migrator Deployment Guide

This guide explains how to deploy the Issue Migrator application to a remote Ubuntu server using Ansible and Docker Compose.

## Prerequisites

### Local Machine Requirements
- Ansible installed (version 2.9 or higher)
- SSH access to the target Ubuntu server
- Git (to clone the repository)

### Target Server Requirements
- Ubuntu 20.04 LTS or 22.04 LTS
- Minimum 2GB RAM
- At least 10GB free disk space
- SSH access enabled
- User with sudo privileges

## Installation Steps

### 1. Install Ansible on Your Local Machine

**macOS:**
```bash
brew install ansible
```

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install ansible
```

**Other systems:**
```bash
pip install ansible
```

### 2. Configure the Inventory File

Edit the `inventory.ini` file to add your server details:

```ini
[ubuntu_servers]
# Replace with your actual server IP and SSH details
192.168.1.100 ansible_user=ubuntu ansible_ssh_private_key_file=~/.ssh/id_rsa
```

**Important:** Replace the following:
- `192.168.1.100` - Your server's IP address or hostname
- `ubuntu` - The SSH username for your server
- `~/.ssh/id_rsa` - Path to your SSH private key

### 3. Test Connection

Verify that Ansible can connect to your server:

```bash
ansible all -i inventory.ini -m ping
```

You should see a SUCCESS response.

### 4. Deploy the Application

Run the deployment script:

```bash
./deploy.sh
```

**Optional flags:**
- `--dry-run` or `--check`: Run in check mode without making changes
- `-v`: Verbose output
- `-vv`: More verbose output
- `-vvv`: Very verbose output

### 5. Verify Deployment

After successful deployment, your application will be accessible at:
- **Frontend**: `http://<your-server-ip>:3000`
- **Backend API**: `http://<your-server-ip>:8080`

## What the Playbook Does

The Ansible playbook performs the following tasks:

1. **System Preparation**
   - Updates apt cache
   - Installs required system packages

2. **Docker Installation**
   - Installs Docker CE and Docker Compose
   - Starts and enables Docker service

3. **Application Deployment**
   - Creates application directory at `/opt/issue-migrator`
   - Copies application files to the server
   - Builds Docker images
   - Starts containers using docker-compose

4. **Health Checks**
   - Waits for backend and frontend to be ready
   - Displays container status and logs

## Manual Deployment Commands

If you prefer to run Ansible directly without the script:

```bash
# Dry run (check mode)
ansible-playbook -i inventory.ini deploy-playbook.yml --check

# Actual deployment
ansible-playbook -i inventory.ini deploy-playbook.yml

# Verbose deployment
ansible-playbook -i inventory.ini deploy-playbook.yml -v
```

## Post-Deployment Management

### View Application Logs

SSH into your server and run:
```bash
cd /opt/issue-migrator
docker-compose logs -f
```

### Stop the Application
```bash
cd /opt/issue-migrator
docker-compose down
```

### Restart the Application
```bash
cd /opt/issue-migrator
docker-compose restart
```

### Update the Application

To deploy updates:
1. Pull the latest changes locally
2. Run the deployment script again: `./deploy.sh`

### View Container Status
```bash
docker ps
```

## Troubleshooting

### Connection Issues

If Ansible cannot connect to your server:
1. Verify SSH access: `ssh ubuntu@your-server-ip`
2. Check SSH key permissions: `chmod 600 ~/.ssh/id_rsa`
3. Debug with: `ansible all -i inventory.ini -m ping -vvv`

### Docker Issues

If Docker containers fail to start:
1. Check Docker status: `sudo systemctl status docker`
2. View container logs: `docker-compose logs`
3. Check disk space: `df -h`

### Port Conflicts

If ports 3000 or 8080 are already in use:
1. Check what's using the ports: `sudo lsof -i :3000` and `sudo lsof -i :8080`
2. Either stop the conflicting services or modify the port mappings in `docker-compose.yml`

### Memory Issues

If the server runs out of memory:
1. Check memory usage: `free -h`
2. Consider adding swap space or upgrading server RAM

## Security Considerations

1. **Firewall**: Configure your server's firewall to only allow necessary ports:
   ```bash
   sudo ufw allow 22/tcp  # SSH
   sudo ufw allow 3000/tcp  # Frontend
   sudo ufw allow 8080/tcp  # Backend API
   sudo ufw enable
   ```

2. **HTTPS**: For production, consider setting up a reverse proxy (nginx) with SSL certificates

3. **Environment Variables**: Store sensitive data (API keys, tokens) in environment variables or secrets management

4. **Updates**: Regularly update Docker and system packages

## Customization

### Change Installation Path

Edit the playbook and set the `app_path` variable:
```yaml
vars:
  app_path: /your/custom/path
```

### Add Environment Variables

Add environment variables in the inventory file:
```ini
[ubuntu_servers:vars]
backend_env_vars=true
```

Then modify the `.env` file section in the playbook as needed.

## Support

For issues or questions:
1. Check the deployment logs
2. Review the troubleshooting section
3. Open an issue in the project repository