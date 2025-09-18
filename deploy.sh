#!/bin/bash

# Issue Migrator Deployment Script
# This script deploys the application using Ansible

set -e

echo "======================================"
echo "Issue Migrator Deployment Script"
echo "======================================"

# Check if ansible is installed
if ! command -v ansible-playbook &> /dev/null; then
    echo "Error: Ansible is not installed."
    echo "Please install Ansible first:"
    echo "  - macOS: brew install ansible"
    echo "  - Ubuntu: sudo apt-get install ansible"
    echo "  - Other: pip install ansible"
    exit 1
fi

# Check if inventory file exists
if [ ! -f "inventory.ini" ]; then
    echo "Error: inventory.ini file not found!"
    echo "Please configure your inventory.ini file with your server details."
    exit 1
fi

# Check if the inventory file has been configured
if grep -q "your-server-ip" inventory.ini; then
    echo "Warning: inventory.ini still contains placeholder values."
    echo "Please edit inventory.ini and replace 'your-server-ip' with your actual server details."
    exit 1
fi

# Parse command line arguments
DRY_RUN=""
CHECK_MODE=""
VERBOSE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run|--check)
            CHECK_MODE="--check"
            echo "Running in check mode (dry run)..."
            ;;
        -v|--verbose)
            VERBOSE="-v"
            ;;
        -vv)
            VERBOSE="-vv"
            ;;
        -vvv)
            VERBOSE="-vvv"
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --dry-run, --check  Run in check mode without making changes"
            echo "  -v, --verbose       Verbose output"
            echo "  -vv                 More verbose output"
            echo "  -vvv                Very verbose output"
            echo "  --help, -h          Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
    shift
done

# Test connection to the server
echo ""
echo "Testing connection to server..."
if ansible all -i inventory.ini -m ping > /dev/null 2>&1; then
    echo "✓ Successfully connected to server"
else
    echo "✗ Failed to connect to server"
    echo "Please check your inventory.ini configuration and SSH access."
    echo ""
    echo "Debug connection with:"
    echo "  ansible all -i inventory.ini -m ping -vvv"
    exit 1
fi

# Run the playbook
echo ""
echo "Starting deployment..."
echo "======================================"

ansible-playbook -i inventory.ini deploy-playbook.yml $CHECK_MODE $VERBOSE

if [ $? -eq 0 ]; then
    echo ""
    echo "======================================"
    echo "✓ Deployment completed successfully!"
    echo "======================================"

    if [ -z "$CHECK_MODE" ]; then
        echo ""
        echo "Your application should now be accessible at:"
        echo "  - Frontend: http://<your-server-ip>:3000"
        echo "  - Backend API: http://<your-server-ip>:8080/api"
        echo ""
        echo "To view logs on the server:"
        echo "  ssh into your server and run: docker-compose logs -f"
    fi
else
    echo ""
    echo "✗ Deployment failed!"
    echo "Please check the error messages above."
    exit 1
fi