# Contributing to Devir

Thank you for your interest in contributing to Devir! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.24 or later
- Make (optional, for using Makefile commands)

### Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/productdevbook/devir.git
   cd devir
   ```

3. Build the project:
   ```bash
   make build
   # or
   go build -o devir ./cmd/devir
   ```

4. Run tests:
   ```bash
   make test
   ```

5. Run linter:
   ```bash
   make lint
   ```

## Project Structure

```
devir/
├── cmd/devir/          # Application entrypoint
│   └── main.go
├── internal/           # Private packages
│   ├── config/         # Configuration loading
│   ├── mcp/            # MCP server implementation
│   ├── runner/         # Service runner
│   ├── tui/            # Terminal UI
│   └── types/          # Shared types
├── .github/workflows/  # CI/CD workflows
├── Makefile            # Build commands
└── README.md
```

## Making Changes

### Code Style

- Follow standard Go conventions
- Run `gofmt` before committing
- Run `golangci-lint run` to check for issues
- Keep functions small and focused
- Add comments for exported functions

### Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Examples:
```
feat(runner): add support for environment variables
fix(tui): resolve viewport scroll issue
docs: update installation instructions
```

### Pull Request Process

1. Create a new branch for your changes:
   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes and commit them following the commit message guidelines

3. Push your branch:
   ```bash
   git push origin feat/my-feature
   ```

4. Open a Pull Request against the `master` branch

5. Ensure CI passes and address any review feedback

### PR Checklist

- [ ] Code follows the project style guidelines
- [ ] Tests added/updated for new functionality
- [ ] Documentation updated if needed
- [ ] Commit messages follow Conventional Commits
- [ ] CI passes

## Reporting Issues

### Bug Reports

When reporting bugs, please include:

- Devir version (`devir -v`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

### Feature Requests

For feature requests, please describe:

- The problem you're trying to solve
- Your proposed solution
- Any alternatives you've considered

## Questions?

Feel free to open an issue for any questions about contributing.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
