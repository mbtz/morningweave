package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func cmdCompletion(args []string) int {
	if len(args) == 0 {
		printCompletionUsage(os.Stdout)
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "-h", "--help", "help":
		printCompletionUsage(os.Stdout)
		return 0
	case "bash":
		fmt.Fprint(os.Stdout, bashCompletionScript())
		return 0
	case "zsh":
		fmt.Fprint(os.Stdout, zshCompletionScript())
		return 0
	case "fish":
		fmt.Fprint(os.Stdout, fishCompletionScript())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown shell: %s\n", args[0])
		printCompletionUsage(os.Stderr)
		return 2
	}
}

func printCompletionUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: morningweave completion <bash|zsh|fish>")
}

func bashCompletionScript() string {
	return `# bash completion for morningweave
_morningweave_complete() {
  local cur prev words cword
  if type _init_completion >/dev/null 2>&1; then
    _init_completion -n : || return
  else
    words=("${COMP_WORDS[@]}")
    cword=$COMP_CWORD
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi

  local commands="init add-platform config completion set-tags set-category run start stop status logs test-email auth cron version"
  local config_subcommands="edit"
  local auth_subcommands="set get clear"

  if [[ $cword -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
    return
  fi

  case "${words[1]}" in
    auth)
      if [[ $cword -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "${auth_subcommands}" -- "${cur}") )
        return
      fi
      ;;
    config)
      if [[ $cword -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "${config_subcommands}" -- "${cur}") )
        return
      fi
      ;;
  esac

  case "${words[1]}" in
    init)
      COMPREPLY=( $(compgen -W "--config --email-provider" -- "${cur}") );;
    add-platform)
      COMPREPLY=( $(compgen -W "--config reddit x instagram hn" -- "${cur}") );;
    config)
      if [[ "${words[2]}" == "edit" ]]; then
        COMPREPLY=( $(compgen -W "--config" -- "${cur}") )
      fi
      ;;
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") );;
    set-tags|set-category)
      COMPREPLY=( $(compgen -W "--config --name --keyword --language --recipient --schedule --weight" -- "${cur}") );;
    run)
      COMPREPLY=( $(compgen -W "--config --tag --category" -- "${cur}") );;
    start)
      COMPREPLY=( $(compgen -W "--config --headless" -- "${cur}") );;
    stop)
      COMPREPLY=( $(compgen -W "--config" -- "${cur}") );;
    status)
      COMPREPLY=( $(compgen -W "--config" -- "${cur}") );;
    logs)
      COMPREPLY=( $(compgen -W "--config --since --json --limit" -- "${cur}") );;
    test-email)
      COMPREPLY=( $(compgen -W "--config --subject" -- "${cur}") );;
    auth)
      case "${words[2]}" in
        set)
          COMPREPLY=( $(compgen -W "--config --ref --value --stdin email reddit x instagram hn" -- "${cur}") );;
        get|clear)
          COMPREPLY=( $(compgen -W "--config email reddit x instagram hn" -- "${cur}") );;
      esac
      ;;
    cron)
      COMPREPLY=( $(compgen -W "--config" -- "${cur}") );;
  esac
}

complete -F _morningweave_complete morningweave
`
}

func zshCompletionScript() string {
	return `#compdef morningweave

autoload -U +X bashcompinit && bashcompinit
source <(morningweave completion bash)
`
}

func fishCompletionScript() string {
	return `# fish completion for morningweave
complete -c morningweave -f -n "__fish_use_subcommand" -a "init add-platform config completion set-tags set-category run start stop status logs test-email auth cron version"

complete -c morningweave -f -n "__fish_seen_subcommand_from config" -a "edit"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth" -a "set get clear"

complete -c morningweave -l config -d "Path to config file"
complete -c morningweave -l email-provider -d "Email provider (resend or smtp)" -n "__fish_seen_subcommand_from init"
complete -c morningweave -l tag -d "Run only this tag" -n "__fish_seen_subcommand_from run"
complete -c morningweave -l category -d "Run only this category" -n "__fish_seen_subcommand_from run"
complete -c morningweave -l headless -d "Run scheduler without prompts" -n "__fish_seen_subcommand_from start"
complete -c morningweave -l since -d "Filter logs since time" -n "__fish_seen_subcommand_from logs"
complete -c morningweave -l json -d "Emit JSON output" -n "__fish_seen_subcommand_from logs"
complete -c morningweave -l limit -d "Maximum runs to display" -n "__fish_seen_subcommand_from logs"
complete -c morningweave -l subject -d "Override email subject" -n "__fish_seen_subcommand_from test-email"

complete -c morningweave -l name -d "Name" -n "__fish_seen_subcommand_from set-tags set-category"
complete -c morningweave -l keyword -d "Keyword (repeatable)" -n "__fish_seen_subcommand_from set-tags set-category"
complete -c morningweave -l language -d "Language filter" -n "__fish_seen_subcommand_from set-tags set-category"
complete -c morningweave -l recipient -d "Recipients" -n "__fish_seen_subcommand_from set-tags set-category"
complete -c morningweave -l schedule -d "Cron schedule override" -n "__fish_seen_subcommand_from set-tags set-category"
complete -c morningweave -l weight -d "Weight override" -n "__fish_seen_subcommand_from set-tags set-category"

complete -c morningweave -l ref -d "Secret reference" -n "__fish_seen_subcommand_from auth set"
complete -c morningweave -l value -d "Secret value" -n "__fish_seen_subcommand_from auth set"
complete -c morningweave -l stdin -d "Read secret from stdin" -n "__fish_seen_subcommand_from auth set"
`
}
