# Contributing to JEND

First off, thank you for considering contributing to JEND! It's people like you that make JEND such a great tool.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct. Please be respectful and considerate of others when contributing.

## How Can I Contribute?

### Reporting Bugs

If you find a bug, please open an issue! We use GitHub issues to track bugs. When opening a new issue, please include:

* **Operating System** (macOS, Linux, Windows)
* **JEND Version** (`jend version`)
* **Network Environment** (Home, School WiFi, Corporate VPN, etc.)
* **Steps to Reproduce**

### Suggesting Enhancements

Have an idea for a new feature? We'd love to hear it. Please open an issue outlining the feature and your use case. JEND values simplicity and reliability, so we carefully evaluate new flags/features to prevent bloat.

### Pull Requests

1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add unit, or e2e tests.
3. If you've changed APIs or commands, update the documentation.
4. Ensure the test suite passes (`make test`).
5. Ensure your code passes the linter (`make lint`).
6. Issue that pull request!

## Development Setup

### CLI Core (Go)

JEND is written in Go (1.23+).

```bash
# Clone the repository
git clone https://github.com/darkprince558/JEND.git
cd JEND

# Download dependencies
make deps

# Build
make build

# Run
make run
```

### Web Receiver (React)

The browser receiver lives in the `web/` directory.

```bash
cd web
npm install --legacy-peer-deps
npm run dev
```

### Backend Infrastructure (AWS CDK)

If you are changing how TURN or Signaling works:

```bash
cd infra
npm install
npx cdk diff
npx cdk deploy
```

## Pull Request Process & Guidelines

* Update the `CHANGELOG.md` with details of changes to the CLI.
* Keep commits focused and atomic.
* Explain the **Why** of a change in your PR description.
* All code should be formatted via standard `gofmt` and lint rules specified in `.golangci.yml`.

Welcome aboard! 🚀
