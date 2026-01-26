# Contributing to HOLOGRAM

Thank you for your interest in contributing to HOLOGRAM!

## Git Workflow

### For Invited Collaborators (Private Repo Access)

If you've been invited as a collaborator to this private repository, please follow this workflow:

> **Important:** Do NOT push directly to `main` or `dev` branches. All changes must go through Pull Requests for review.

1. **Clone the repository** (if you haven't already)
   ```bash
   git clone https://github.com/DHEBP/HOLOGRAM.git
   cd HOLOGRAM
   ```

2. **Create a feature branch from `dev`**
   ```bash
   git checkout dev
   git pull origin dev
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**
   - Follow existing code style
   - Test your changes locally
   - Update documentation if needed

4. **Commit your changes**
   ```bash
   git commit -m "Description of changes"
   ```

5. **Push your branch**
   ```bash
   git push origin feature/your-feature-name
   ```

6. **Create a Pull Request**
   - Go to GitHub and create a PR from your branch to `dev`
   - Provide a clear description of changes
   - Reference any related issues
   - Wait for review and approval before merge

### Branch Naming Conventions

Use descriptive branch names with prefixes:
- `feature/` - New features or enhancements
- `fix/` - Bug fixes
- `docs/` - Documentation updates
- `refactor/` - Code refactoring

Examples:
- `feature/add-wallet-export`
- `fix/gnomon-sync-issue`
- `docs/update-telahost-api`
- `refactor/explorer-service`

## Development Setup

### Prerequisites

- **Go** 1.21+
- **Wails** v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Node.js** 18+

### Running Locally

```bash
cd frontend && npm install && cd ..
wails dev
```

### Building

```bash
wails build
```

## Code Guidelines

### Go Code

- Follow standard Go conventions
- Run `go fmt` before committing
- Add comments for exported functions
- Handle errors appropriately

### Frontend (Svelte)

- Keep components focused and reusable
- Use existing styling patterns
- Test UI changes across different states

### Testing

Before submitting a PR:

1. Run the app locally with `wails dev`
2. Test the specific feature/fix you implemented
3. Verify no regressions in related functionality
4. Build with `wails build` to ensure it compiles

## Reporting Issues

- Check existing issues before creating a new one
- Provide clear descriptions and steps to reproduce bugs
- Include relevant logs or screenshots
- Specify your OS and HOLOGRAM version

## Questions?

- Open a GitHub issue for questions or discussions
- Join the DERO Discord community

## License

By contributing, you agree that your contributions will be licensed under the project's license.
