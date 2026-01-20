# Prompt
- Writing your result in the USER_TODO.md file.
- Check if you can use the `git add PROMPT.md`, `git commit -m "chore: test git commands"`, and `git push` commands.
  - If all commands succeed, write `We DID it!` in USER_TODO.md.
  - If you cannot, please verify which gitconfig you have active in your shell by write the result of the commands below:
    ```zsh
      git config --show-origin --get commit.gpgsign
      git config --show-origin --get gpg.ssh.program
      git config --show-origin --get user.signingkey
    ```
  - Write out the .codex/config.toml you are using
  - If git add/commit still fails, append a short "Git debug bundle" to USER_TODO.md with outputs of:
    ```zsh
      pwd
      echo "HOME=$HOME"
      echo "CODEX_HOME=$CODEX_HOME"
      echo "CODEX_CONFIG=$CODEX_CONFIG"
      echo "CODEX_SANDBOX=$CODEX_SANDBOX"
      env | rg -n '^(HOME|CODEX|FORGE|XDG|GIT|SSH)='
      git rev-parse --show-toplevel
      git rev-parse --git-dir
      ls -ldO .git
      ls -la .git | head -n 5
      touch .git/index.lock && rm .git/index.lock
      git config --show-origin --get commit.gpgsign
      git config --show-origin --get gpg.ssh.program
      git config --show-origin --get user.signingkey
      git add -A
      git commit -m "chore: test git commands" 2>&1 | sed -n '1,120p'
    ```
