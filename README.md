# ghp

Terminal UI for GitHub Projects v2.

## Install

```bash
go install github.com/h0rv/ghp/cmd/ghp@latest
```

Or build from source:

```bash
git clone https://github.com/h0rv/ghp.git
cd ghp
go build -o ghp ./cmd/ghp
```

## Authentication

Requires a GitHub token with `project` scope.

**Option 1** - GitHub CLI (recommended):
```bash
gh auth login
```

**Option 2** - Environment variable:
```bash
export GITHUB_TOKEN=ghp_your_token_here
```

## Usage

```bash
ghp                                    # Interactive mode
ghp --owner myorg                      # Skip owner prompt
ghp --owner myorg --project 1          # Skip project picker
```

Run `ghp --help` for all options. Press `?` in the app for keybindings.

## License

MIT
