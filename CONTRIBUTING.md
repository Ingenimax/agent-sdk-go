# Contributing to the Agent SDK

Thank you for your interest in contributing to the Agent SDK! This document provides guidelines and instructions for contributing to this project.

## Code of Conduct

Please be respectful to all contributors and users. We aim to foster an open and welcoming environment.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** to your local machine
3. **Create a branch** for your changes
4. **Make your changes** and test them
5. **Push your branch** to your fork
6. **Create a pull request**

## Development Environment

1. Install Go (version 1.21 or later recommended)
2. Set up your IDE with Go support (GoLand, VSCode with Go extensions, etc.)
3. Install required dependencies:
   ```bash
   go mod download
   ```

## Code Style

We follow standard Go code style and conventions:

1. Use `gofmt` or `goimports` to format your code
2. Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
3. Document all exported types, functions, and methods

## Testing

Please include tests for any new functionality or bug fixes:

1. Unit tests should be added for all new functions and methods
2. Integration tests should be added for significant components
3. Run tests before submitting a pull request:
   ```bash
   go test ./...
   ```

## Pull Request Process

1. Ensure your code passes all tests and linting checks
2. Update documentation to reflect any changes
3. Include a clear description of the changes in your pull request
4. Reference any related issues in your pull request

## Adding New Features

When adding new features, please follow these guidelines:

1. **Discuss before implementing**: Open an issue to discuss significant new features before implementing them
2. **Be consistent**: Follow the existing architecture and patterns
3. **Documentation**: Add documentation for all new features
4. **Examples**: Add examples showing how to use new features

## Reporting Bugs

When reporting bugs, please include:

1. A clear description of the issue
2. Steps to reproduce the bug
3. Expected and actual behavior
4. Environment details (Go version, OS, etc.)

## License

By contributing to this project, you agree that your contributions will be licensed under the project's license.

Thank you for contributing to the Agent SDK! 