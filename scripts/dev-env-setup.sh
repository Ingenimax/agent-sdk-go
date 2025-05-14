#!/bin/bash
set -euo pipefail

echo "Installing pre-commit hooks..."

# Check if pre-commit is installed
if ! command -v pre-commit &> /dev/null; then
    echo "pre-commit not found. Installing..."

    # Check package manager and install pre-commit
    if command -v pip &> /dev/null; then
        pip install pre-commit
    elif command -v brew &> /dev/null; then
        brew install pre-commit
    else
        echo "Error: Neither pip nor brew found. Please install pre-commit manually:"
        echo "https://pre-commit.com/#installation"
        exit 1
    fi
fi

# Check if golangci-lint is installed
if ! command -v golangci-lint &> /dev/null; then
    echo "golangci-lint not found. Installing..."

    # Check if we can use the go install method
    if command -v go &> /dev/null; then
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    elif command -v brew &> /dev/null; then
        brew install golangci-lint
    else
        echo "Error: Neither go nor brew found. Please install golangci-lint manually:"
        echo "https://golangci-lint.run/usage/install/"
        exit 1
    fi
fi

# Check if gosec is installed
if ! command -v gosec &> /dev/null; then
    echo "gosec not found. Installing..."

    # Check if we can use the go install method
    if command -v go &> /dev/null; then
        go install github.com/securego/gosec/v2/cmd/gosec@latest
    elif command -v brew &> /dev/null; then
        brew install gosec
    else
        echo "Error: Neither go nor brew found. Please install gosec manually:"
        echo "https://github.com/securego/gosec#installation"
        exit 1
    fi
fi

# Install the pre-commit hooks
pre-commit install

echo "Pre-commit hooks installed successfully!"
echo "These hooks will run automatically on each commit."
echo "To run the hooks manually on all files: pre-commit run --all-files"
echo ""
echo "Note: For a custom golangci-lint configuration, create a .golangci.yml file"
echo "in your project root. See https://golangci-lint.run/usage/configuration/ for examples."
echo "For gosec configuration, see https://github.com/securego/gosec#configuration"
