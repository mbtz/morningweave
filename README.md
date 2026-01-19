# MorningWeave CLI (Go)

MorningWeave is a single-user CLI that builds a scheduled content digest from official platform APIs.
The Go CLI is the source of truth. Legacy Python packaging has been removed; use git history if needed.

## Quick start
- Install via Homebrew (recommended):
  - `brew tap mbtz/morningweave`
  - `brew install morningweave`
  - If you want latest `main` without a release: `brew install --HEAD morningweave`
- Or install to PATH: `./scripts/install.sh` (or `go install ./cmd/morningweave`)
- Help: `morningweave --help`
- Init config: `morningweave init`

## Requirements
- Go 1.24+ (module-managed; toolchain auto-download is enabled)
- macOS (primary), but intended to run on a home server as well

## Configuration overview
- `config.yaml` is created by `init` (defaults are safe and editable).
- Secrets should be stored in the OS keychain or 1Password (plaintext is a fallback).
- Plaintext fallback lives under `secrets.values` and can be referenced via `secrets:<key>` or `env:VAR_NAME`.
- Platforms can define `weight` and per-source `source_weights` to influence ranking.

## Config examples
Example platform snippets to drop into `config.yaml`:

Reddit:
```yaml
platforms:
  reddit:
    enabled: true
    credentials_ref: "keychain:reddit"
    sources:
      subreddits: ["golang", "machinelearning"]
      users: ["spez"]
      keywords: ["golang", "llm"]
    source_weights:
      subreddits:
        golang: 1.2
```

X (x.com):
```yaml
platforms:
  x:
    enabled: true
    credentials_ref: "keychain:x"
    sources:
      users: ["OpenAI"]
      lists: ["1234567890"]
      keywords: ["golang", "machine learning"]
```

Instagram:
```yaml
platforms:
  instagram:
    enabled: true
    credentials_ref: "keychain:instagram"
    sources:
      accounts: ["openai"]
      hashtags: ["ai", "golang"]
```
Note: Instagram Graph API requires a Business or Creator account linked to a Facebook Page/app.

Hacker News:
```yaml
platforms:
  hn:
    enabled: true
    sources:
      lists: ["top", "best", "new"]
      keywords: ["golang", "llm"]
```

## Implemented commands
The following commands are implemented today:

- `init` - generate `config.yaml`
- `add-platform <name>` - enable a platform and capture sources/creds ref
- `config edit` - open `config.yaml` in your default terminal editor
- `completion <shell>` - emit shell completion script (bash, zsh, fish)
- `set-tags` - add/update tag definitions in `config.yaml`
- `set-category` - add/update category definitions in `config.yaml`
- `run [--tag <tag>|--category <cat>]` - execute a one-shot digest run
- `start --headless` / `stop` - run the background scheduler loop and stop it
- `cron` - emit crontab entries for the configured schedules
- `status` - show enabled platforms and next scheduled runs (last run recorded if available)
- `logs [--since <time>]` - show recent run history
- `test-email` - send a sample digest to verify email delivery
- `auth set|get|clear <platform|email>` - manage secret references without printing values

Examples:

```sh
morningweave init --email-provider resend
morningweave add-platform reddit
morningweave set-tags --name "ai" --keyword "llm" --keyword "machine learning" --language en
morningweave set-category --name "work" --keyword "golang" --recipient "you@example.com"
```

## Project docs
- `PRD.md` is the product specification.
- `docs/platform-setup.md` is the step-by-step setup guide for platforms and keys.

## Shell completion
Generate completion scripts and source them in your shell config:

```sh
# Bash
morningweave completion bash > ~/.morningweave-completion.bash
echo 'source ~/.morningweave-completion.bash' >> ~/.bashrc

# Zsh
morningweave completion zsh > ~/.morningweave-completion.zsh
echo 'source ~/.morningweave-completion.zsh' >> ~/.zshrc

# Fish
morningweave completion fish > ~/.config/fish/completions/morningweave.fish
```

## Development
- Run tests: `go test ./...`

## Release (Homebrew)
You can run `./scripts/update-version.sh` to automate the steps below.

0) Bump the version you want to release (optional but recommended):
   - Update `tap/Formula/morningweave.rb` (`version` + `url` tag).
   - If you want `morningweave --version` to show the release by default, update `internal/cli/cli.go` (`Version`).

1) Create an annotated tag and push it:
```sh
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```
2) Compute the tarball checksum and update the formula:
```sh
curl -L https://github.com/mbtz/morningweave/archive/refs/tags/v1.0.0.tar.gz | shasum -a 256
```
Paste the checksum into `tap/Formula/morningweave.rb` (`sha256`) and confirm `version`/`url` match.

3) Commit the formula update and push:
```sh
git -C tap add Formula/morningweave.rb
git -C tap commit -m "Update Homebrew formula for v1.0.0"
git -C tap push
```
