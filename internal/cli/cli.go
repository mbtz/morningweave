package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"morningweave/internal/config"
	"morningweave/internal/connectors"
	"morningweave/internal/dedupe"
	"morningweave/internal/email"
	"morningweave/internal/runner"
	"morningweave/internal/scaffold"
	"morningweave/internal/schedule"
	"morningweave/internal/scheduler"
	"morningweave/internal/secrets"
	"morningweave/internal/storage"
)

type handler func(args []string) int

var Version = "1.1.0"

var commands = []string{
	"init",
	"add-platform",
	"config",
	"completion",
	"set-tags",
	"set-category",
	"run",
	"start",
	"stop",
	"status",
	"logs",
	"test-email",
	"auth",
	"cron",
	"version",
}

type platformSpec struct {
	sources    []string
	needsCreds bool
}

var platformSpecs = map[string]platformSpec{
	"reddit":    {sources: []string{"subreddits", "users", "keywords"}, needsCreds: true},
	"x":         {sources: []string{"users", "keywords", "lists"}, needsCreds: true},
	"instagram": {sources: []string{"accounts", "hashtags"}, needsCreds: true},
	"hn":        {sources: []string{"lists", "keywords"}, needsCreds: false},
}

func Run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return 0
	case "-v", "--version", "version":
		fmt.Fprintln(os.Stdout, Version)
		return 0
	case "init":
		return cmdInit(args[1:])
	case "add-platform":
		return cmdAddPlatform(args[1:])
	case "config":
		return cmdConfig(args[1:])
	case "completion":
		return cmdCompletion(args[1:])
	case "set-tags":
		return cmdSetTags(args[1:])
	case "set-category":
		return cmdSetCategory(args[1:])
	case "status":
		return cmdStatus(args[1:])
	case "logs":
		return cmdLogs(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "test-email":
		return cmdTestEmail(args[1:])
	case "auth":
		return cmdAuth(args[1:])
	case "start":
		return cmdStart(args[1:])
	case "stop":
		return cmdStop(args[1:])
	case "cron":
		return cmdCron(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "MorningWeave async content digest CLI.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  morningweave <command> [options]")
	fmt.Fprintln(w, "  morningweave -v | --version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	for _, cmd := range commands {
		fmt.Fprintf(w, "  %s\n", cmd)
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `morningweave <command> --help` for command-specific options.")
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	emailProvider := fs.String("email-provider", "", "Email provider for the generated config (resend or smtp).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printInitUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printInitUsage(os.Stderr, fs)
		return 2
	}

	provider := strings.TrimSpace(*emailProvider)
	if provider == "" {
		if isInteractive() {
			provider = promptEmailProvider(os.Stdin, os.Stdout)
		} else {
			provider = scaffold.DefaultEmailProvider
		}
	}

	normalized := scaffold.NormalizeEmailProvider(provider)
	if provider != "" && normalized != strings.ToLower(provider) {
		fmt.Fprintf(os.Stderr, "Unknown provider '%s', using '%s'.\n", provider, scaffold.DefaultEmailProvider)
	}

	result, err := scaffold.InitWorkspace(*configPath, normalized, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init workspace: %v\n", err)
		return 1
	}

	for _, path := range result.Created {
		fmt.Printf("created %s\n", path)
	}
	for _, path := range result.Skipped {
		fmt.Printf("exists %s\n", path)
	}

	if len(result.Created) == 0 {
		return 1
	}
	return 0
}

func cmdAddPlatform(args []string) int {
	fs := flag.NewFlagSet("add-platform", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printAddPlatformUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printAddPlatformUsage(os.Stderr, fs)
		return 2
	}

	if fs.NArg() < 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fmt.Fprintln(os.Stderr, "platform name is required.")
		printAddPlatformUsage(os.Stderr, fs)
		return 2
	}

	name := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	spec, ok := platformSpecs[name]
	if !ok {
		options := []string{}
		for key := range platformSpecs {
			options = append(options, key)
		}
		sortStrings(options)
		fmt.Fprintf(os.Stderr, "Unknown platform '%s'. Expected one of: %s.\n", fs.Arg(0), strings.Join(options, ", "))
		return 1
	}

	config, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	platformsValue := config["platforms"]
	platforms := map[string]any{}
	if platformsValue == nil {
		config["platforms"] = platforms
	} else {
		var ok bool
		platforms, ok = coerceStringMap(platformsValue)
		if !ok {
			fmt.Fprintln(os.Stderr, "platforms must be a mapping/object in config.yaml.")
			return 1
		}
		config["platforms"] = platforms
	}

	platformValue := platforms[name]
	platform := map[string]any{}
	if platformValue == nil {
		platforms[name] = platform
	} else {
		var ok bool
		platform, ok = coerceStringMap(platformValue)
		if !ok {
			fmt.Fprintf(os.Stderr, "platforms.%s must be a mapping/object in config.yaml.\n", name)
			return 1
		}
		platforms[name] = platform
	}

	platform["enabled"] = true

	if spec.needsCreds {
		credentialsRef := normalizeString(platform["credentials_ref"])
		if credentialsRef == "" {
			credentialsRef = fmt.Sprintf("keychain:%s", name)
		}
		if isInteractive() {
			setupNow, ok := promptConfirm(os.Stdin, os.Stdout, "Configure credentials now", true)
			if ok && setupNow {
				ref, err := setupPlatformCredentials(os.Stdin, os.Stdout, name, config, credentialsRef)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				if strings.TrimSpace(ref) != "" {
					credentialsRef = ref
				}
			} else {
				selection, ok := promptText(os.Stdin, os.Stdout, "Credentials reference", credentialsRef)
				if ok && strings.TrimSpace(selection) != "" {
					credentialsRef = selection
				}
			}
		}
		platform["credentials_ref"] = credentialsRef
	}

	sourcesValue := platform["sources"]
	sources := map[string]any{}
	if sourcesValue == nil {
		platform["sources"] = sources
	} else {
		var ok bool
		sources, ok = coerceStringMap(sourcesValue)
		if !ok {
			fmt.Fprintf(os.Stderr, "platforms.%s.sources must be a mapping/object in config.yaml.\n", name)
			return 1
		}
		platform["sources"] = sources
	}

	for _, sourceKey := range spec.sources {
		existing := normalizeListValue(sources[sourceKey])
		update, updated := promptList(os.Stdin, os.Stdout, fmt.Sprintf("%s %s", name, sourceKey), existing)
		if !updated {
			if _, exists := sources[sourceKey]; !exists {
				sources[sourceKey] = existing
			}
			continue
		}
		sources[sourceKey] = dedupeStrings(append(existing, update...))
	}

	if err := writeYAMLConfig(*configPath, config); err != nil {
		return 1
	}

	fmt.Printf("platform enabled: %s\n", name)
	if !isInteractive() {
		fmt.Println("non-interactive mode: edit config.yaml to add sources/creds.")
	}
	return 0
}

func cmdConfig(args []string) int {
	if len(args) == 0 {
		printConfigUsage(os.Stdout)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		printConfigUsage(os.Stdout)
		return 0
	case "edit":
		return cmdConfigEdit(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config command: %s\n", args[0])
		printConfigUsage(os.Stderr)
		return 2
	}
}

func cmdConfigEdit(args []string) int {
	fs := flag.NewFlagSet("config edit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printConfigEditUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printConfigEditUsage(os.Stderr, fs)
		return 2
	}

	if _, err := os.Stat(*configPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to stat config: %v\n", err)
		return 1
	}

	editor := selectEditor()
	command := fmt.Sprintf("%s %s", editor, shellEscape(*configPath))
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to launch editor: %v\n", err)
		return 1
	}
	return 0
}

func cmdCompletion(args []string) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCompletionUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printCompletionUsage(os.Stderr, fs)
		return 2
	}

	if fs.NArg() < 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fmt.Fprintln(os.Stderr, "shell name is required.")
		printCompletionUsage(os.Stderr, fs)
		return 2
	}

	shell := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	switch shell {
	case "bash":
		fmt.Fprint(os.Stdout, completionScriptBash())
	case "zsh":
		fmt.Fprint(os.Stdout, completionScriptZsh())
	case "fish":
		fmt.Fprint(os.Stdout, completionScriptFish())
	case "-h", "--help", "help":
		printCompletionUsage(os.Stdout, fs)
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell %q. Use bash, zsh, or fish.\n", shell)
		return 2
	}
	return 0
}

func cmdSetTags(args []string) int {
	return cmdSetLabel(args, "tags")
}

func cmdSetCategory(args []string) int {
	return cmdSetLabel(args, "categories")
}

func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printStatusUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printStatusUsage(os.Stdout, fs)
		return 2
	}

	cfgMap, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	typedCfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	enabledPlatforms := collectEnabledPlatforms(cfgMap)
	globalSchedule := ""
	if globalSection, ok := coerceStringMap(cfgMap["global"]); ok {
		globalSchedule = normalizeString(globalSection["default_schedule"])
	}

	now := time.Now()
	fmt.Fprintf(os.Stdout, "Config: %s\n", *configPath)
	if len(enabledPlatforms) == 0 {
		fmt.Fprintln(os.Stdout, "Enabled platforms: none")
	} else {
		fmt.Fprintf(os.Stdout, "Enabled platforms: %s\n", strings.Join(enabledPlatforms, ", "))
	}

	fmt.Fprintln(os.Stdout, "Next runs:")
	printScheduleLine(os.Stdout, "global", globalSchedule, now)
	for _, label := range collectLabelSchedules(cfgMap, "tags", globalSchedule) {
		printScheduleLine(os.Stdout, fmt.Sprintf("tag %q", label.Name), label.Schedule, now)
	}
	for _, label := range collectLabelSchedules(cfgMap, "categories", globalSchedule) {
		printScheduleLine(os.Stdout, fmt.Sprintf("category %q", label.Name), label.Schedule, now)
	}

	warnings := collectStatusWarnings(typedCfg)
	if len(warnings) > 0 {
		fmt.Fprintln(os.Stdout, "Warnings:")
		for _, warning := range warnings {
			fmt.Fprintf(os.Stdout, "  - %s\n", warning)
		}
	}

	storagePath := scaffold.DefaultStoragePath
	if storageSection, ok := coerceStringMap(cfgMap["storage"]); ok {
		if path := normalizeString(storageSection["path"]); path != "" {
			storagePath = path
		}
	}

	if _, err := os.Stat(storagePath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stdout, "Last run: none recorded")
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to stat storage: %v\n", err)
		return 1
	}

	db, err := storage.Open(storagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open storage: %v\n", err)
		return 1
	}
	defer db.Close()

	lastRun, ok, err := storage.GetLastRun(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load last run: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintln(os.Stdout, "Last run: none recorded")
		return 0
	}

	fmt.Fprintf(os.Stdout, "Last run: %s\n", formatRunLogLine(lastRun))
	return 0
}

func cmdLogs(args []string) int {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	since := fs.String("since", "", "Filter logs since time (RFC3339, YYYY-MM-DD, or duration like 24h).")
	jsonOutput := fs.Bool("json", false, "Emit JSON output.")
	limit := fs.Int("limit", 50, "Maximum number of runs to display.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printLogsUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printLogsUsage(os.Stdout, fs)
		return 2
	}

	config, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	storagePath := scaffold.DefaultStoragePath
	if storageSection, ok := coerceStringMap(config["storage"]); ok {
		if path := normalizeString(storageSection["path"]); path != "" {
			storagePath = path
		}
	}

	if _, err := os.Stat(storagePath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stdout, "No runs recorded (database not found at %s).\n", storagePath)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to stat storage: %v\n", err)
		return 1
	}

	db, err := storage.Open(storagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open storage: %v\n", err)
		return 1
	}
	defer db.Close()

	var sinceTime time.Time
	if strings.TrimSpace(*since) != "" {
		parsed, err := parseSinceTime(*since, time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid --since: %v\n", err)
			return 1
		}
		sinceTime = parsed
	}

	var records []storage.RunRecord
	if !sinceTime.IsZero() {
		records, err = storage.ListRunsSince(db, sinceTime, *limit)
	} else {
		records, err = storage.ListRuns(db, *limit)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load runs: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(os.Stdout, "No runs recorded.")
		return 0
	}

	if *jsonOutput {
		if err := writeRunLogsJSON(os.Stdout, records); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write json: %v\n", err)
			return 1
		}
		return 0
	}

	for _, record := range records {
		fmt.Fprintln(os.Stdout, formatRunLogLine(record))
	}
	return 0
}

func cmdTestEmail(args []string) int {
	fs := flag.NewFlagSet("test-email", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	subject := fs.String("subject", "", "Override subject for the test email.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printTestEmailUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printTestEmailUsage(os.Stderr, fs)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	from := strings.TrimSpace(cfg.Email.From)
	if from == "" {
		fmt.Fprintln(os.Stderr, "email.from is required.")
		return 1
	}
	recipients := cfg.Email.To
	if len(recipients) == 0 {
		fmt.Fprintln(os.Stderr, "email.to must include at least one recipient.")
		return 1
	}

	now := time.Now()
	subjectValue := strings.TrimSpace(*subject)
	if subjectValue == "" {
		subjectValue = renderSubject(cfg.Email.Subject, now)
	}
	if subjectValue == "" {
		subjectValue = "MorningWeave Test Email"
	}

	items := sampleDigestItems(now)
	rendered, err := email.RenderDigest(items, email.RenderOptions{
		Title:       subjectValue,
		WordCap:     cfg.Global.Digest.WordCap,
		MaxItems:    3,
		GeneratedAt: now,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to render test digest: %v\n", err)
		return 1
	}

	resolver := secrets.NewResolver(cfg.Secrets.Values)
	sender, warnings, err := email.NewSenderFromConfig(cfg.Email, resolver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure email sender: %v\n", err)
		return 1
	}

	if len(warnings) > 0 {
		fmt.Fprintln(os.Stdout, "Warnings:")
		for _, warning := range warnings {
			fmt.Fprintf(os.Stdout, "  - %s\n", warning)
		}
	}

	if err := sender.Send(context.Background(), email.Message{
		From:    from,
		To:      recipients,
		Subject: subjectValue,
		HTML:    rendered.HTML,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to send test email: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stdout, "Test email sent.")
	return 0
}

func cmdAuth(args []string) int {
	if len(args) == 0 {
		printAuthUsage(os.Stdout)
		return 2
	}

	switch args[0] {
	case "set":
		return cmdAuthSet(args[1:])
	case "get":
		return cmdAuthGet(args[1:])
	case "clear":
		return cmdAuthClear(args[1:])
	case "-h", "--help", "help":
		printAuthUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown auth command: %s\n", args[0])
		printAuthUsage(os.Stderr)
		return 2
	}
}

func cmdAuthSet(args []string) int {
	fs := flag.NewFlagSet("auth set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	refInput := fs.String("ref", "", "Secret reference to store (default: secrets:<target>).")
	value := fs.String("value", "", "Secret value (omit to read from stdin or prompt).")
	readStdin := fs.Bool("stdin", false, "Read secret value from stdin.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printAuthSetUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printAuthSetUsage(os.Stderr, fs)
		return 2
	}

	if fs.NArg() < 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fmt.Fprintln(os.Stderr, "auth target is required (platform or email).")
		printAuthSetUsage(os.Stderr, fs)
		return 2
	}

	if *readStdin && strings.TrimSpace(*value) != "" {
		fmt.Fprintln(os.Stderr, "choose either --value or --stdin, not both.")
		return 2
	}

	config, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	target := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	refInfo, err := resolveAuthRef(config, target, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !refInfo.NeedsAuth {
		fmt.Fprintf(os.Stdout, "%s does not require authentication.\n", refInfo.Label())
		return 0
	}

	ref := strings.TrimSpace(*refInput)
	if ref == "" {
		ref = refInfo.Ref
	}
	if ref == "" {
		ref = defaultAuthRef(refInfo)
	}
	ref = normalizeAuthRef(ref)

	provider, key, ok := secrets.ParseRef(ref)
	if !ok || strings.TrimSpace(key) == "" {
		fmt.Fprintln(os.Stderr, "invalid reference; use secrets:<key>, keychain:<key>, env:VAR, or op://vault/item/field.")
		return 2
	}

	if provider == "op" || provider == "1password" {
		refInfo.SetRef(ref)
		if err := writeYAMLConfig(*configPath, config); err != nil {
			return 1
		}
		fmt.Fprintf(os.Stdout, "Stored 1Password reference for %s: %s\n", refInfo.Label(), ref)
		fmt.Fprintln(os.Stdout, "1Password is read-only; ensure the item and field key exist.")
		return 0
	}

	secretValue := strings.TrimSpace(*value)
	if *readStdin {
		secret, err := readSecretValue(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read secret from stdin: %v\n", err)
			return 1
		}
		secretValue = secret
	}
	if secretValue == "" {
		if entered, ok := promptSecret(os.Stdin, os.Stdout, "Secret value"); ok {
			secretValue = entered
		}
	}
	if secretValue == "" {
		fmt.Fprintln(os.Stderr, "secret value is required.")
		return 2
	}

	values, ok := ensureSecretsValues(config)
	if !ok {
		return 1
	}

	store := secrets.NewStore(values)
	if _, err := store.Set(ref, secretValue); err != nil {
		if errors.Is(err, secrets.ErrReadOnlyProvider) {
			fmt.Fprintf(os.Stderr, "provider %q is read-only.\n", provider)
			return 1
		}
		if errors.Is(err, secrets.ErrProviderUnavailable) {
			fmt.Fprintf(os.Stderr, "provider %q is unavailable; install its CLI or configure secrets:<key>.\n", provider)
			return 1
		}
		if errors.Is(err, secrets.ErrUnsupportedProvider) {
			fmt.Fprintf(os.Stderr, "provider %q is unsupported.\n", provider)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to store secret: %v\n", err)
		return 1
	}

	refInfo.SetRef(ref)
	if err := writeYAMLConfig(*configPath, config); err != nil {
		return 1
	}

	fmt.Fprintf(os.Stdout, "Stored secret for %s (ref %s).\n", refInfo.Label(), ref)
	return 0
}

func cmdAuthGet(args []string) int {
	fs := flag.NewFlagSet("auth get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printAuthGetUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printAuthGetUsage(os.Stderr, fs)
		return 2
	}

	if fs.NArg() < 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fmt.Fprintln(os.Stderr, "auth target is required (platform or email).")
		printAuthGetUsage(os.Stderr, fs)
		return 2
	}

	config, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	target := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	refInfo, err := resolveAuthRef(config, target, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !refInfo.NeedsAuth {
		fmt.Fprintf(os.Stdout, "%s does not require authentication.\n", refInfo.Label())
		return 0
	}
	if strings.TrimSpace(refInfo.Ref) == "" {
		fmt.Fprintf(os.Stderr, "no secret reference configured for %s.\n", refInfo.Label())
		return 1
	}

	values, ok := getSecretsValues(config)
	if !ok {
		return 1
	}
	store := secrets.NewStore(values)
	status, err := store.Inspect(refInfo.Ref)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "missing secret for %s (ref %s).\n", refInfo.Label(), refInfo.Ref)
			return 1
		}
		if errors.Is(err, secrets.ErrProviderUnavailable) {
			fmt.Fprintf(os.Stderr, "provider %q is unavailable; install its CLI or use secrets:<key>.\n", status.Provider)
			return 1
		}
		if errors.Is(err, secrets.ErrUnsupportedProvider) {
			fmt.Fprintf(os.Stderr, "provider %q is not supported yet.\n", status.Provider)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to check secret: %v\n", err)
		return 1
	}

	note := ""
	if status.ReadOnly {
		note = " (read-only)"
	}
	fmt.Fprintf(os.Stdout, "Found secret for %s (ref %s, provider %s)%s.\n", refInfo.Label(), refInfo.Ref, status.Provider, note)
	return 0
}

func cmdAuthClear(args []string) int {
	fs := flag.NewFlagSet("auth clear", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printAuthClearUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printAuthClearUsage(os.Stderr, fs)
		return 2
	}

	if fs.NArg() < 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fmt.Fprintln(os.Stderr, "auth target is required (platform or email).")
		printAuthClearUsage(os.Stderr, fs)
		return 2
	}

	config, ok := loadYAMLConfig(*configPath)
	if !ok {
		return 1
	}

	target := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	refInfo, err := resolveAuthRef(config, target, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !refInfo.NeedsAuth {
		fmt.Fprintf(os.Stdout, "%s does not require authentication.\n", refInfo.Label())
		return 0
	}
	if strings.TrimSpace(refInfo.Ref) == "" {
		fmt.Fprintf(os.Stderr, "no secret reference configured for %s.\n", refInfo.Label())
		return 1
	}

	values, ok := getSecretsValues(config)
	if !ok {
		return 1
	}
	store := secrets.NewStore(values)
	_, err = store.Clear(refInfo.Ref)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			fmt.Fprintf(os.Stdout, "No stored secret for %s (ref %s).\n", refInfo.Label(), refInfo.Ref)
			return 0
		}
		if errors.Is(err, secrets.ErrReadOnlyProvider) {
			fmt.Fprintf(os.Stderr, "provider %q is read-only.\n", normalizeProvider(refInfo.Ref))
			return 1
		}
		if errors.Is(err, secrets.ErrProviderUnavailable) {
			fmt.Fprintf(os.Stderr, "provider %q is unavailable; install its CLI or use secrets:<key>.\n", normalizeProvider(refInfo.Ref))
			return 1
		}
		if errors.Is(err, secrets.ErrUnsupportedProvider) {
			fmt.Fprintf(os.Stderr, "provider %q is not supported yet.\n", normalizeProvider(refInfo.Ref))
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to clear secret: %v\n", err)
		return 1
	}

	if err := writeYAMLConfig(*configPath, config); err != nil {
		return 1
	}

	fmt.Fprintf(os.Stdout, "Cleared secret for %s (ref %s).\n", refInfo.Label(), refInfo.Ref)
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	tag := fs.String("tag", "", "Run only the named tag.")
	category := fs.String("category", "", "Run only the named category.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRunUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printRunUsage(os.Stderr, fs)
		return 2
	}

	if strings.TrimSpace(*tag) != "" && strings.TrimSpace(*category) != "" {
		fmt.Fprintln(os.Stderr, "choose either --tag or --category, not both.")
		printRunUsage(os.Stderr, fs)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	scope := runner.RunScope{Type: "global"}

	if name := strings.TrimSpace(*tag); name != "" {
		tagConfig, ok := cfg.FindTag(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "tag %q not found.\n", name)
			printLabelNames(os.Stdout, "Available tags", tagNames(cfg.Tags))
			return 1
		}
		scope = runner.RunScope{
			Type:       "tag",
			Name:       tagConfig.Name,
			Keywords:   tagConfig.Keywords,
			Languages:  tagConfig.Language,
			Weight:     tagConfig.Weight,
			Recipients: tagConfig.Recipients,
		}
	}

	if name := strings.TrimSpace(*category); name != "" {
		categoryConfig, ok := cfg.FindCategory(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "category %q not found.\n", name)
			printLabelNames(os.Stdout, "Available categories", tagNames(cfg.Categories))
			return 1
		}
		scope = runner.RunScope{
			Type:       "category",
			Name:       categoryConfig.Name,
			Keywords:   categoryConfig.Keywords,
			Languages:  categoryConfig.Language,
			Weight:     categoryConfig.Weight,
			Recipients: categoryConfig.Recipients,
		}
	}

	result, err := runner.RunOnce(context.Background(), cfg, runner.RunOptions{Scope: scope})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		return 1
	}

	if issues := runner.DetectAccessIssues(result.Warnings); len(issues) > 0 {
		if _, err := config.DisablePlatforms(*configPath, issues); err != nil {
			fmt.Fprintf(os.Stderr, "failed to disable platforms: %v\n", err)
		}
		platforms := make([]string, 0, len(issues))
		for platform := range issues {
			platforms = append(platforms, platform)
		}
		sort.Strings(platforms)
		for _, platform := range platforms {
			reason := strings.TrimSpace(issues[platform])
			if reason == "" {
				reason = "access unavailable"
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: disabled due to access issue (%s)", platform, reason))
		}
	}

	fmt.Fprintf(os.Stdout, "Run status: %s\n", result.Record.Status)
	fmt.Fprintf(os.Stdout, "Items fetched: %d\n", result.Record.ItemsFetched)
	fmt.Fprintf(os.Stdout, "Items ranked: %d\n", result.Record.ItemsRanked)
	fmt.Fprintf(os.Stdout, "Items in digest: %d\n", result.Record.ItemsSent)

	if len(result.Warnings) > 0 {
		fmt.Fprintln(os.Stdout, "Warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(os.Stdout, "  - %s\n", warning)
		}
	}

	if result.Record.Status == "success" {
		if result.Record.EmailSent {
			fmt.Fprintln(os.Stdout, "Email sent.")
		} else {
			fmt.Fprintln(os.Stdout, "Email not sent.")
		}
	}

	return 0
}

func cmdStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	headless := fs.Bool("headless", false, "Run scheduler loop without interactive prompts.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printStartUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printStartUsage(os.Stderr, fs)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	storagePath := strings.TrimSpace(cfg.Storage.Path)
	if storagePath == "" {
		storagePath = scaffold.DefaultStoragePath
	}

	control := scheduler.PathsForStorage(storagePath)
	if pid, err := scheduler.ReadPID(control.PID); err == nil {
		if scheduler.ProcessRunning(pid) {
			fmt.Fprintf(os.Stderr, "scheduler already running (pid %d).\n", pid)
			return 1
		}
		scheduler.RemovePID(control.PID)
	}

	if *headless {
		fmt.Fprintln(os.Stdout, "Starting scheduler in headless mode. Use `morningweave stop` to stop it.")
	} else {
		fmt.Fprintln(os.Stdout, "Starting scheduler. Press Ctrl+C to stop.")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err = scheduler.RunLoop(ctx, scheduler.Options{
		ConfigPath:   *configPath,
		Logger:       os.Stdout,
		PollInterval: time.Minute,
		Control:      control,
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "scheduler stopped with error: %v\n", err)
		return 1
	}
	return 0
}

func cmdStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printStopUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printStopUsage(os.Stderr, fs)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	storagePath := strings.TrimSpace(cfg.Storage.Path)
	if storagePath == "" {
		storagePath = scaffold.DefaultStoragePath
	}
	control := scheduler.PathsForStorage(storagePath)

	if err := scheduler.RequestStop(control.Stop); err != nil {
		fmt.Fprintf(os.Stderr, "failed to request stop: %v\n", err)
		return 1
	}

	if pid, err := scheduler.ReadPID(control.PID); err == nil {
		_ = scheduler.SignalStop(pid)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(control.PID); os.IsNotExist(err) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		fmt.Fprintf(os.Stdout, "Stop requested for scheduler (pid %d).\n", pid)
		return 0
	}

	fmt.Fprintln(os.Stdout, "Stop requested. No active scheduler pid file found.")
	return 0
}

func cmdCron(args []string) int {
	fs := flag.NewFlagSet("cron", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	command := fs.String("command", "morningweave", "Command to invoke in cron (absolute path recommended).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCronUsage(os.Stdout, fs)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printCronUsage(os.Stderr, fs)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", *configPath)
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	entries, warnings := buildCronEntries(cfg, *configPath, *command)
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	now := time.Now().Format(time.RFC3339)
	fmt.Fprintf(os.Stdout, "# MorningWeave cron entries (generated %s)\n", now)
	fmt.Fprintf(os.Stdout, "# Update with: morningweave cron --config %s\n", shellQuote(*configPath))
	fmt.Fprintln(os.Stdout, "")

	if len(entries) == 0 {
		fmt.Fprintln(os.Stdout, "# No cron entries generated. Check schedules in config.yaml.")
		return 0
	}

	for _, entry := range entries {
		if entry.Comment != "" {
			fmt.Fprintf(os.Stdout, "# %s\n", entry.Comment)
		}
		fmt.Fprintf(os.Stdout, "%s %s\n", entry.Schedule, entry.Command)
	}
	return 0
}

type labelSchedule struct {
	Name     string
	Schedule string
}

func collectStatusWarnings(cfg config.Config) []string {
	warnings := []string{}
	store := secrets.NewStore(cfg.Secrets.Values)

	provider := strings.ToLower(strings.TrimSpace(cfg.Email.Provider))
	if provider == "" {
		warnings = append(warnings, "email: provider is not configured")
	}
	if strings.TrimSpace(cfg.Email.From) == "" {
		warnings = append(warnings, "email: from is not configured")
	}
	if len(cfg.Email.To) == 0 {
		warnings = append(warnings, "email: no recipients configured")
	}

	switch provider {
	case "resend":
		warnings, _ = appendSecretWarning(warnings, "email.resend.api_key_ref", cfg.Email.Resend.APIKeyRef, store)
	case "smtp":
		if strings.TrimSpace(cfg.Email.SMTP.Host) == "" {
			warnings = append(warnings, "email.smtp.host is required")
		}
		warnings, _ = appendSecretWarning(warnings, "email.smtp.password_ref", cfg.Email.SMTP.PasswordRef, store)
	case "":
		// no-op; already warned above.
	default:
		warnings = append(warnings, fmt.Sprintf("email: unsupported provider %q", provider))
	}

	platforms := []struct {
		name       string
		cfg        *config.PlatformConfig
		needsCreds bool
	}{
		{name: "reddit", cfg: cfg.Platforms.Reddit, needsCreds: true},
		{name: "x", cfg: cfg.Platforms.X, needsCreds: true},
		{name: "instagram", cfg: cfg.Platforms.Instagram, needsCreds: true},
		{name: "hn", cfg: cfg.Platforms.HN, needsCreds: false},
	}

	for _, platform := range platforms {
		if platform.cfg == nil {
			continue
		}
		if !platform.cfg.Enabled {
			if strings.TrimSpace(platform.cfg.DisabledReason) != "" {
				warnings = append(warnings, fmt.Sprintf("%s: disabled (%s)", platform.name, platform.cfg.DisabledReason))
			}
			continue
		}
		if !platformHasSources(platform.cfg) {
			warnings = append(warnings, fmt.Sprintf("%s: enabled but no sources configured", platform.name))
		}
		if platform.needsCreds {
			label := fmt.Sprintf("platforms.%s.credentials_ref", platform.name)
			var missing bool
			warnings, missing = appendSecretWarning(warnings, label, platform.cfg.CredentialsRef, store)
			if missing {
				warnings = runner.AppendAuthRequirementHint(warnings, platform.name)
			}
		}
	}

	return warnings
}

func appendSecretWarning(warnings []string, label string, ref string, store secrets.Store) ([]string, bool) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return append(warnings, fmt.Sprintf("%s is required", label)), true
	}
	status, err := store.Inspect(trimmed)
	if err == nil {
		if status.Found {
			return warnings, false
		}
		return append(warnings, fmt.Sprintf("%s missing (ref %s)", label, trimmed)), true
	}

	switch {
	case errors.Is(err, secrets.ErrNotFound):
		return append(warnings, fmt.Sprintf("%s missing (ref %s)", label, trimmed)), true
	case errors.Is(err, secrets.ErrProviderUnavailable):
		return append(warnings, fmt.Sprintf("%s provider unavailable (ref %s)", label, trimmed)), true
	case errors.Is(err, secrets.ErrUnsupportedProvider):
		return append(warnings, fmt.Sprintf("%s unsupported provider (ref %s)", label, trimmed)), true
	default:
		return append(warnings, fmt.Sprintf("%s check failed (ref %s): %v", label, trimmed, err)), true
	}
}

func platformHasSources(cfg *config.PlatformConfig) bool {
	if cfg == nil {
		return false
	}
	for _, list := range cfg.Sources {
		for _, entry := range list {
			if strings.TrimSpace(entry) != "" {
				return true
			}
		}
	}
	return false
}

func collectEnabledPlatforms(config map[string]any) []string {
	platforms := []string{}
	platformSection, ok := coerceStringMap(config["platforms"])
	if !ok {
		return platforms
	}

	for name, raw := range platformSection {
		entry, ok := coerceStringMap(raw)
		if !ok {
			continue
		}
		if parseBool(entry["enabled"]) {
			platforms = append(platforms, name)
		}
	}
	sortStrings(platforms)
	return platforms
}

func collectLabelSchedules(config map[string]any, key string, fallback string) []labelSchedule {
	labelsValue := config[key]
	items, ok := labelsValue.([]any)
	if !ok {
		return nil
	}

	result := make([]labelSchedule, 0, len(items))
	for _, raw := range items {
		entry, ok := coerceStringMap(raw)
		if !ok {
			continue
		}
		name := normalizeString(entry["name"])
		if name == "" {
			continue
		}
		schedule := normalizeString(entry["schedule"])
		if schedule == "" {
			schedule = fallback
		}
		result = append(result, labelSchedule{
			Name:     name,
			Schedule: schedule,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func tagNames(tags []config.TagConfig) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag.Name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	sort.Strings(names)
	return names
}

func printLabelNames(w io.Writer, label string, names []string) {
	if len(names) == 0 {
		fmt.Fprintf(w, "%s: none\n", label)
		return
	}
	fmt.Fprintf(w, "%s: %s\n", label, strings.Join(names, ", "))
}

func printScheduleLine(w io.Writer, label string, scheduleValue string, now time.Time) {
	if strings.TrimSpace(scheduleValue) == "" {
		fmt.Fprintf(w, "  %s: no schedule configured\n", label)
		return
	}
	next, err := schedule.NextRun(scheduleValue, now)
	if err != nil {
		fmt.Fprintf(w, "  %s: invalid schedule (%s)\n", label, scheduleValue)
		return
	}
	fmt.Fprintf(w, "  %s (%s): %s\n", label, scheduleValue, formatRunTime(next))
}

func formatRunTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.Local().Format("2006-01-02 15:04")
}

func renderSubject(template string, now time.Time) string {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, "{{date}}", now.Format("2006-01-02"))
}

func parseSinceTime(value string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, nil
	}
	if duration, err := time.ParseDuration(trimmed); err == nil {
		return now.Add(-duration), nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
	}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, trimmed, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected RFC3339, YYYY-MM-DD, or duration like 24h")
}

func formatRunLogLine(record storage.RunRecord) string {
	started := formatRunTime(record.StartedAt)
	scope := formatRunScope(record.ScopeType, record.ScopeName)
	platforms := formatPlatformCounts(record.PlatformCounts)
	segments := []string{
		started,
		record.Status,
	}
	if scope != "" {
		segments = append(segments, scope)
	}
	segments = append(segments, fmt.Sprintf("items=%d/%d/%d", record.ItemsFetched, record.ItemsRanked, record.ItemsSent))
	segments = append(segments, fmt.Sprintf("email=%s", boolToYesNo(record.EmailSent)))
	if platforms != "" {
		segments = append(segments, "platforms="+platforms)
	}
	if record.Error != "" {
		segments = append(segments, "error="+sanitizeInline(record.Error))
	}
	return strings.Join(segments, " ")
}

func formatRunScope(scopeType string, scopeName string) string {
	if scopeType == "" && scopeName == "" {
		return ""
	}
	if scopeType == "" {
		return "scope=" + scopeName
	}
	if scopeName == "" {
		return "scope=" + scopeType
	}
	return fmt.Sprintf("scope=%s:%s", scopeType, scopeName)
}

func formatPlatformCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}

func boolToYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func sanitizeInline(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	trimmed = strings.ReplaceAll(trimmed, "\r", " ")
	return trimmed
}

type authRef struct {
	Target    string
	Provider  string
	Ref       string
	NeedsAuth bool
	container map[string]any
	key       string
}

func (a authRef) Label() string {
	if a.Target == "email" && a.Provider != "" {
		return fmt.Sprintf("email (%s)", a.Provider)
	}
	return a.Target
}

func (a authRef) SetRef(value string) {
	if a.container == nil || a.key == "" {
		return
	}
	a.container[a.key] = value
	a.Ref = value
}

func resolveAuthRef(config map[string]any, target string, allowCreate bool) (authRef, error) {
	if strings.TrimSpace(target) == "" {
		return authRef{}, errors.New("auth target is required")
	}
	if target == "email" {
		emailSection, ok := coerceStringMap(config["email"])
		if !ok {
			if allowCreate && config["email"] == nil {
				emailSection = map[string]any{}
				config["email"] = emailSection
			} else {
				return authRef{}, errors.New("email section is missing or invalid in config.yaml")
			}
		}

		provider := normalizeString(emailSection["provider"])
		if provider == "" {
			return authRef{}, errors.New("email.provider is required")
		}

		switch provider {
		case "resend":
			resendSection, ok := coerceStringMap(emailSection["resend"])
			if !ok {
				if allowCreate && emailSection["resend"] == nil {
					resendSection = map[string]any{}
					emailSection["resend"] = resendSection
				} else {
					return authRef{}, errors.New("email.resend must be a mapping/object in config.yaml")
				}
			}
			return authRef{
				Target:    "email",
				Provider:  provider,
				Ref:       normalizeString(resendSection["api_key_ref"]),
				NeedsAuth: true,
				container: resendSection,
				key:       "api_key_ref",
			}, nil
		case "smtp":
			smtpSection, ok := coerceStringMap(emailSection["smtp"])
			if !ok {
				if allowCreate && emailSection["smtp"] == nil {
					smtpSection = map[string]any{}
					emailSection["smtp"] = smtpSection
				} else {
					return authRef{}, errors.New("email.smtp must be a mapping/object in config.yaml")
				}
			}
			return authRef{
				Target:    "email",
				Provider:  provider,
				Ref:       normalizeString(smtpSection["password_ref"]),
				NeedsAuth: true,
				container: smtpSection,
				key:       "password_ref",
			}, nil
		default:
			return authRef{}, fmt.Errorf("unsupported email provider: %s", provider)
		}
	}

	spec, ok := platformSpecs[target]
	if !ok {
		return authRef{}, fmt.Errorf("unknown auth target: %s", target)
	}
	if !spec.needsCreds {
		return authRef{Target: target, NeedsAuth: false}, nil
	}

	platformsSection, ok := coerceStringMap(config["platforms"])
	if !ok {
		if allowCreate && config["platforms"] == nil {
			platformsSection = map[string]any{}
			config["platforms"] = platformsSection
		} else {
			return authRef{}, errors.New("platforms must be a mapping/object in config.yaml")
		}
	}

	platformSection, ok := coerceStringMap(platformsSection[target])
	if !ok {
		if allowCreate && platformsSection[target] == nil {
			platformSection = map[string]any{}
			platformsSection[target] = platformSection
		} else {
			return authRef{}, fmt.Errorf("platforms.%s must be a mapping/object in config.yaml", target)
		}
	}

	return authRef{
		Target:    target,
		Provider:  target,
		Ref:       normalizeString(platformSection["credentials_ref"]),
		NeedsAuth: true,
		container: platformSection,
		key:       "credentials_ref",
	}, nil
}

func defaultAuthRef(ref authRef) string {
	key := ref.Target
	if ref.Target == "email" && ref.Provider != "" {
		key = ref.Provider
	}
	return fmt.Sprintf("secrets:%s", key)
}

func normalizeAuthRef(ref string) string {
	provider, key, ok := secrets.ParseRef(ref)
	if !ok {
		return ref
	}
	if provider == "plain" && strings.TrimSpace(key) != "" {
		return fmt.Sprintf("secrets:%s", key)
	}
	return ref
}

func normalizeProvider(ref string) string {
	provider, _, ok := secrets.ParseRef(ref)
	if !ok {
		return ""
	}
	return provider
}

type runLogOutput struct {
	ID             int64          `json:"id"`
	StartedAt      string         `json:"started_at"`
	FinishedAt     string         `json:"finished_at,omitempty"`
	Status         string         `json:"status"`
	ScopeType      string         `json:"scope_type,omitempty"`
	ScopeName      string         `json:"scope_name,omitempty"`
	ItemsFetched   int            `json:"items_fetched"`
	ItemsRanked    int            `json:"items_ranked"`
	ItemsSent      int            `json:"items_sent"`
	EmailSent      bool           `json:"email_sent"`
	Error          string         `json:"error,omitempty"`
	PlatformCounts map[string]int `json:"platform_counts,omitempty"`
	CreatedAt      string         `json:"created_at,omitempty"`
}

func writeRunLogsJSON(w io.Writer, records []storage.RunRecord) error {
	output := make([]runLogOutput, 0, len(records))
	for _, record := range records {
		entry := runLogOutput{
			ID:             record.ID,
			StartedAt:      record.StartedAt.Format(time.RFC3339),
			Status:         record.Status,
			ScopeType:      record.ScopeType,
			ScopeName:      record.ScopeName,
			ItemsFetched:   record.ItemsFetched,
			ItemsRanked:    record.ItemsRanked,
			ItemsSent:      record.ItemsSent,
			EmailSent:      record.EmailSent,
			Error:          record.Error,
			PlatformCounts: record.PlatformCounts,
		}
		if !record.FinishedAt.IsZero() {
			entry.FinishedAt = record.FinishedAt.Format(time.RFC3339)
		}
		if !record.CreatedAt.IsZero() {
			entry.CreatedAt = record.CreatedAt.Format(time.RFC3339)
		}
		output = append(output, entry)
	}

	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(encoded))
	return err
}

func parseBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		return normalized == "true" || normalized == "yes" || normalized == "1"
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func cmdSetLabel(args []string, labelKey string) int {
	fs := flag.NewFlagSet(labelKey, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", scaffold.DefaultConfigFilename, "Path to config file (default: config.yaml).")
	name := fs.String("name", "", "Name")
	keywords := multiString{}
	languages := multiString{}
	recipients := multiString{}
	schedule := optionalString{}
	weight := optionalFloat64{}
	fs.Var(&keywords, "keyword", "Keyword(s), repeatable or comma-separated")
	fs.Var(&languages, "language", "Language(s), repeatable or comma-separated")
	fs.Var(&recipients, "recipient", "Recipient emails, repeatable or comma-separated")
	fs.Var(&schedule, "schedule", "Override cron schedule (5 fields)")
	fs.Var(&weight, "weight", "Weight override (positive)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printSetLabelUsage(os.Stdout, fs, labelKey)
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		printSetLabelUsage(os.Stderr, fs, labelKey)
		return 2
	}

	trimmedName := strings.TrimSpace(*name)
	if trimmedName == "" {
		fmt.Fprintln(os.Stderr, "--name is required.")
		printSetLabelUsage(os.Stderr, fs, labelKey)
		return 2
	}

	keywordList := normalizeListArgs(keywords.values)
	if len(keywordList) == 0 {
		fmt.Fprintln(os.Stderr, "At least one keyword is required.")
		return 1
	}

	var scheduleValue *string
	if schedule.set {
		value := strings.TrimSpace(schedule.value)
		if !validateSchedule(value) {
			fmt.Fprintln(os.Stderr, "schedule must be a 5-field cron expression.")
			return 1
		}
		scheduleValue = &value
	}

	var weightValue *float64
	if weight.set {
		if weight.value <= 0 {
			fmt.Fprintln(os.Stderr, "weight must be positive.")
			return 1
		}
		value := weight.value
		weightValue = &value
	}

	return setLabel(
		*configPath,
		labelKey,
		trimmedName,
		keywordList,
		scheduleValue,
		normalizeListArgs(languages.values),
		normalizeListArgs(recipients.values),
		weightValue,
	)
}

func printAddPlatformUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave add-platform [options] <name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Interactive runs prompt for sources and credential setup.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "See docs/platform-setup.md for step-by-step key setup.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printConfigUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: morningweave config <command> [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  edit    Open config.yaml in your terminal editor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `morningweave config <command> --help` for command-specific options.")
}

func printConfigEditUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave config edit [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printCompletionUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave completion <shell>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Shells:")
	fmt.Fprintln(w, "  bash, zsh, fish")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printSetLabelUsage(w io.Writer, fs *flag.FlagSet, labelKey string) {
	usageName := "set-tags"
	if labelKey == "categories" {
		usageName = "set-category"
	}
	fmt.Fprintf(w, "Usage: morningweave %s [options]\n", usageName)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printStatusUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave status [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printLogsUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave logs [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printRunUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave run [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printStartUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave start [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printStopUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave stop [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printCronUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave cron [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func completionScriptBash() string {
	return `# bash completion for morningweave
_morningweave() {
  local cur prev
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  local commands="init add-platform config completion set-tags set-category run start stop status logs test-email auth cron version"

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
    return 0
  fi

  case "${COMP_WORDS[1]}" in
    init)
      COMPREPLY=( $(compgen -W "--config --email-provider --help -h" -- "$cur") )
      ;;
    add-platform)
      COMPREPLY=( $(compgen -W "--config --help -h reddit x instagram hn" -- "$cur") )
      ;;
    config)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "edit --help -h" -- "$cur") )
      else
        case "${COMP_WORDS[2]}" in
          edit)
            COMPREPLY=( $(compgen -W "--config --help -h" -- "$cur") )
            ;;
        esac
      fi
      ;;
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish --help -h" -- "$cur") )
      ;;
    set-tags|set-category)
      COMPREPLY=( $(compgen -W "--config --name --keyword --language --recipient --schedule --weight --help -h" -- "$cur") )
      ;;
    run)
      COMPREPLY=( $(compgen -W "--config --tag --category --help -h" -- "$cur") )
      ;;
    start)
      COMPREPLY=( $(compgen -W "--config --headless --help -h" -- "$cur") )
      ;;
    stop)
      COMPREPLY=( $(compgen -W "--config --help -h" -- "$cur") )
      ;;
    status)
      COMPREPLY=( $(compgen -W "--config --help -h" -- "$cur") )
      ;;
    logs)
      COMPREPLY=( $(compgen -W "--config --since --json --limit --help -h" -- "$cur") )
      ;;
    test-email)
      COMPREPLY=( $(compgen -W "--config --subject --help -h" -- "$cur") )
      ;;
    auth)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "set get clear --help -h" -- "$cur") )
      else
        case "${COMP_WORDS[2]}" in
          set)
            COMPREPLY=( $(compgen -W "--config --ref --value --stdin --help -h x reddit instagram hn email" -- "$cur") )
            ;;
          get|clear)
            COMPREPLY=( $(compgen -W "--config --help -h x reddit instagram hn email" -- "$cur") )
            ;;
        esac
      fi
      ;;
    cron)
      COMPREPLY=( $(compgen -W "--config --command --help -h" -- "$cur") )
      ;;
  esac
}
complete -F _morningweave morningweave
`
}

func completionScriptZsh() string {
	return `#compdef morningweave

local -a commands
commands=(init add-platform config completion set-tags set-category run start stop status logs test-email auth cron version)

if (( CURRENT == 2 )); then
  compadd -- $commands
  return
fi

case $words[2] in
  init)
    compadd -- --config --email-provider --help -h
    ;;
  add-platform)
    compadd -- --config --help -h reddit x instagram hn
    ;;
  config)
    if (( CURRENT == 3 )); then
      compadd -- edit --help -h
    else
      case $words[3] in
        edit)
          compadd -- --config --help -h
          ;;
      esac
    fi
    ;;
  completion)
    compadd -- bash zsh fish --help -h
    ;;
  set-tags|set-category)
    compadd -- --config --name --keyword --language --recipient --schedule --weight --help -h
    ;;
  run)
    compadd -- --config --tag --category --help -h
    ;;
  start)
    compadd -- --config --headless --help -h
    ;;
  stop)
    compadd -- --config --help -h
    ;;
  status)
    compadd -- --config --help -h
    ;;
  logs)
    compadd -- --config --since --json --limit --help -h
    ;;
  test-email)
    compadd -- --config --subject --help -h
    ;;
  auth)
    if (( CURRENT == 3 )); then
      compadd -- set get clear --help -h
    else
      case $words[3] in
        set)
          compadd -- --config --ref --value --stdin --help -h x reddit instagram hn email
          ;;
        get|clear)
          compadd -- --config --help -h x reddit instagram hn email
          ;;
      esac
    fi
    ;;
  cron)
    compadd -- --config --command --help -h
    ;;
esac
`
}

func completionScriptFish() string {
	return `complete -c morningweave -f -n "__fish_use_subcommand" -a "init add-platform config completion set-tags set-category run start stop status logs test-email auth cron version"

complete -c morningweave -f -n "__fish_seen_subcommand_from init" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from init" -l email-provider -d "Email provider"

complete -c morningweave -f -n "__fish_seen_subcommand_from add-platform" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from add-platform" -a "reddit x instagram hn"

complete -c morningweave -f -n "__fish_seen_subcommand_from config" -a "edit"
complete -c morningweave -f -n "__fish_seen_subcommand_from config; and __fish_seen_subcommand_from edit" -l config -d "Path to config file"

complete -c morningweave -f -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"

complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l name -d "Name"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l keyword -d "Keyword"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l language -d "Language"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l recipient -d "Recipient"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l schedule -d "Schedule"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-tags" -l weight -d "Weight"

complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l name -d "Name"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l keyword -d "Keyword"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l language -d "Language"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l recipient -d "Recipient"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l schedule -d "Schedule"
complete -c morningweave -f -n "__fish_seen_subcommand_from set-category" -l weight -d "Weight"

complete -c morningweave -f -n "__fish_seen_subcommand_from run" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from run" -l tag -d "Tag"
complete -c morningweave -f -n "__fish_seen_subcommand_from run" -l category -d "Category"

complete -c morningweave -f -n "__fish_seen_subcommand_from start" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from start" -l headless -d "Headless mode"

complete -c morningweave -f -n "__fish_seen_subcommand_from stop" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from status" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from logs" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from logs" -l since -d "Since time"
complete -c morningweave -f -n "__fish_seen_subcommand_from logs" -l json -d "JSON output"
complete -c morningweave -f -n "__fish_seen_subcommand_from logs" -l limit -d "Limit"

complete -c morningweave -f -n "__fish_seen_subcommand_from test-email" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from test-email" -l subject -d "Subject"

complete -c morningweave -f -n "__fish_seen_subcommand_from auth" -a "set get clear"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from set" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from set" -l ref -d "Secret reference"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from set" -l value -d "Secret value"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from set" -l stdin -d "Read from stdin"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from set" -a "x reddit instagram hn email"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from get" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from get" -a "x reddit instagram hn email"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from clear" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from auth; and __fish_seen_subcommand_from clear" -a "x reddit instagram hn email"

complete -c morningweave -f -n "__fish_seen_subcommand_from cron" -l config -d "Path to config file"
complete -c morningweave -f -n "__fish_seen_subcommand_from cron" -l command -d "Command"
`
}

func printAuthUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: morningweave auth <set|get|clear> [options] <platform|email>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "See docs/platform-setup.md for step-by-step key setup.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Targets:")
	fmt.Fprintln(w, "  email, reddit, x, instagram, hn")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  set    Store a secret reference without printing values.")
	fmt.Fprintln(w, "  get    Check secret availability (no values printed).")
	fmt.Fprintln(w, "  clear  Remove a stored secret from local secrets.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `morningweave auth <subcommand> --help` for subcommand options.")
}

func printAuthSetUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave auth set [options] <platform|email>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Note: Use --ref op://<vault>/<item>/<field> for 1Password references.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printAuthGetUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave auth get [options] <platform|email>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printAuthClearUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave auth clear [options] <platform|email>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printTestEmailUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave test-email [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func printInitUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "Usage: morningweave init [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func promptEmailProvider(input io.Reader, output io.Writer) string {
	choices := strings.Join(scaffold.ValidEmailProviders(), "/")
	fmt.Fprintf(output, "Email provider [%s] (default %s): ", choices, scaffold.DefaultEmailProvider)

	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return scaffold.DefaultEmailProvider
	}

	selection := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if selection == "" {
		return scaffold.DefaultEmailProvider
	}

	return selection
}

type multiString struct {
	values []string
}

func (m *multiString) String() string {
	return strings.Join(m.values, ",")
}

func (m *multiString) Set(value string) error {
	m.values = append(m.values, value)
	return nil
}

type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string {
	return o.value
}

func (o *optionalString) Set(value string) error {
	o.value = value
	o.set = true
	return nil
}

type optionalFloat64 struct {
	value float64
	set   bool
}

func (o *optionalFloat64) String() string {
	return fmt.Sprintf("%v", o.value)
}

func (o *optionalFloat64) Set(value string) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	o.value = parsed
	o.set = true
	return nil
}

func promptText(input io.Reader, output io.Writer, prompt string, defaultValue string) (string, bool) {
	if !isInteractive() {
		return "", false
	}
	suffix := ""
	if defaultValue != "" {
		suffix = fmt.Sprintf(" [%s]", defaultValue)
	}
	fmt.Fprintf(output, "%s%s: ", prompt, suffix)

	reader := bufio.NewReader(input)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", false
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return defaultValue, true
	}
	return value, true
}

func promptSecret(input io.Reader, output io.Writer, prompt string) (string, bool) {
	return promptText(input, output, prompt, "")
}

func readSecretValue(input io.Reader) (string, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func promptList(input io.Reader, output io.Writer, prompt string, existing []string) ([]string, bool) {
	if !isInteractive() {
		return nil, false
	}
	suffix := ""
	if len(existing) > 0 {
		suffix = fmt.Sprintf(" (current: %s)", strings.Join(existing, ", "))
	}
	fmt.Fprintf(output, "%s (comma-separated, blank to keep)%s: ", prompt, suffix)

	reader := bufio.NewReader(input)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return nil, false
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return nil, false
	}
	return normalizeListArgs([]string{value}), true
}

func promptConfirm(input io.Reader, output io.Writer, prompt string, defaultYes bool) (bool, bool) {
	if !isInteractive() {
		return false, false
	}
	defaultValue := "y"
	if !defaultYes {
		defaultValue = "n"
	}
	answer, ok := promptText(input, output, fmt.Sprintf("%s (y/n)", prompt), defaultValue)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, true
	case "n", "no":
		return false, true
	default:
		return defaultYes, true
	}
}

func setupPlatformCredentials(input io.Reader, output io.Writer, platform string, config map[string]any, existingRef string) (string, error) {
	if !isInteractive() {
		return existingRef, nil
	}
	defaultProvider := normalizeProvider(existingRef)
	if defaultProvider == "" {
		defaultProvider = "keychain"
	}
	provider, ok := promptCredentialProvider(input, output, defaultProvider)
	if !ok {
		return existingRef, nil
	}
	switch provider {
	case "skip":
		return existingRef, nil
	case "op":
		ref, ok := promptOpReference(input, output, platform, existingRef)
		if !ok || strings.TrimSpace(ref) == "" {
			return "", errors.New("1Password reference is required")
		}
		fmt.Fprintf(output, "Set 1Password reference: %s\n", ref)
		return ref, nil
	case "keychain":
		key := defaultRefKey(existingRef, platform, "keychain")
		selection, ok := promptText(input, output, "Keychain account (service/account)", key)
		if !ok || strings.TrimSpace(selection) == "" {
			return "", errors.New("keychain account is required")
		}
		ref := fmt.Sprintf("keychain:%s", selection)
		payload, err := promptPlatformCredentialsPayload(input, output, platform)
		if err != nil {
			return "", err
		}
		store := secrets.NewStore(nil)
		if _, err := store.Set(ref, payload); err != nil {
			return "", mapStoreError(provider, err)
		}
		return ref, nil
	case "secrets":
		key := defaultRefKey(existingRef, platform, "secrets")
		selection, ok := promptText(input, output, "Secrets key", key)
		if !ok || strings.TrimSpace(selection) == "" {
			return "", errors.New("secrets key is required")
		}
		ref := fmt.Sprintf("secrets:%s", selection)
		payload, err := promptPlatformCredentialsPayload(input, output, platform)
		if err != nil {
			return "", err
		}
		values, ok := ensureSecretsValues(config)
		if !ok {
			return "", errors.New("secrets.values is unavailable in config.yaml")
		}
		store := secrets.NewStore(values)
		if _, err := store.Set(ref, payload); err != nil {
			return "", mapStoreError(provider, err)
		}
		return ref, nil
	default:
		return existingRef, nil
	}
}

func promptCredentialProvider(input io.Reader, output io.Writer, defaultProvider string) (string, bool) {
	normalizedDefault := normalizeCredentialProvider(defaultProvider)
	if normalizedDefault == "" {
		normalizedDefault = "keychain"
	}
	value, ok := promptText(input, output, "Credentials provider (keychain/1password/secrets/skip)", normalizedDefault)
	if !ok {
		return "", false
	}
	normalized := normalizeCredentialProvider(value)
	if normalized == "" {
		fmt.Fprintf(output, "Unknown provider %q. Choose keychain, 1password, secrets, or skip.\n", value)
		return "", false
	}
	return normalized, true
}

func normalizeCredentialProvider(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "keychain", "kc":
		return "keychain"
	case "secrets", "secret":
		return "secrets"
	case "1password", "op", "onepassword", "1p":
		return "op"
	case "skip", "later", "none":
		return "skip"
	default:
		return ""
	}
}

func promptOpReference(input io.Reader, output io.Writer, platform string, existingRef string) (string, bool) {
	vault, item, field := parseOpRef(existingRef)
	if strings.TrimSpace(field) == "" && platform == "x" {
		field = "x-api-key"
	}

	vault, ok := promptText(input, output, "1Password vault", vault)
	if !ok {
		return "", false
	}
	item, ok = promptText(input, output, "1Password item", item)
	if !ok {
		return "", false
	}
	fieldPrompt := "1Password field"
	if platform == "x" {
		fieldPrompt = "1Password field (x-api-key or x-ap-key)"
	}
	field, ok = promptText(input, output, fieldPrompt, field)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(vault) == "" || strings.TrimSpace(item) == "" || strings.TrimSpace(field) == "" {
		return "", false
	}
	return fmt.Sprintf("op://%s/%s/%s", strings.TrimSpace(vault), strings.TrimSpace(item), strings.TrimSpace(field)), true
}

func parseOpRef(ref string) (string, string, string) {
	if strings.TrimSpace(ref) == "" {
		return "", "", ""
	}
	provider, key, ok := secrets.ParseRef(ref)
	if !ok || provider != "op" {
		return "", "", ""
	}
	raw := strings.TrimPrefix(key, "op://")
	parts := strings.Split(raw, "/")
	if len(parts) < 3 {
		return "", "", ""
	}
	vault := strings.TrimSpace(parts[0])
	item := strings.TrimSpace(parts[1])
	field := strings.TrimSpace(strings.Join(parts[2:], "/"))
	return vault, item, field
}

func defaultRefKey(existingRef string, fallback string, provider string) string {
	if strings.TrimSpace(existingRef) == "" {
		return fallback
	}
	refProvider, key, ok := secrets.ParseRef(existingRef)
	if !ok {
		return fallback
	}
	if refProvider == provider && strings.TrimSpace(key) != "" {
		return key
	}
	return fallback
}

func promptPlatformCredentialsPayload(input io.Reader, output io.Writer, platform string) (string, error) {
	switch platform {
	case "x":
		token, ok := promptSecret(input, output, "X bearer token")
		if !ok || strings.TrimSpace(token) == "" {
			return "", errors.New("x bearer token is required")
		}
		return strings.TrimSpace(token), nil
	case "reddit":
		clientID, ok := promptText(input, output, "Reddit client id", "")
		if !ok {
			return "", errors.New("reddit client id is required")
		}
		clientSecret, _ := promptText(input, output, "Reddit client secret (optional)", "")
		userAgent, ok := promptText(input, output, "Reddit user agent", "")
		if !ok {
			return "", errors.New("reddit user agent is required")
		}
		username, _ := promptText(input, output, "Reddit username (optional)", "")
		password, _ := promptSecret(input, output, "Reddit password (optional)")
		if strings.TrimSpace(clientID) == "" || strings.TrimSpace(userAgent) == "" {
			return "", errors.New("client_id and user_agent are required")
		}
		payload := map[string]string{
			"client_id":  strings.TrimSpace(clientID),
			"user_agent": strings.TrimSpace(userAgent),
		}
		if strings.TrimSpace(clientSecret) != "" {
			payload["client_secret"] = strings.TrimSpace(clientSecret)
		}
		if strings.TrimSpace(username) != "" {
			payload["username"] = strings.TrimSpace(username)
		}
		if strings.TrimSpace(password) != "" {
			payload["password"] = strings.TrimSpace(password)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to encode reddit credentials: %w", err)
		}
		return string(data), nil
	case "instagram":
		token, ok := promptSecret(input, output, "Instagram access token")
		if !ok || strings.TrimSpace(token) == "" {
			return "", errors.New("instagram access token is required")
		}
		userID, _ := promptText(input, output, "Instagram user id (optional)", "")
		payload := map[string]string{
			"access_token": strings.TrimSpace(token),
		}
		if strings.TrimSpace(userID) != "" {
			payload["user_id"] = strings.TrimSpace(userID)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to encode instagram credentials: %w", err)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
}

func mapStoreError(provider string, err error) error {
	switch {
	case errors.Is(err, secrets.ErrReadOnlyProvider):
		return fmt.Errorf("provider %q is read-only", provider)
	case errors.Is(err, secrets.ErrProviderUnavailable):
		return fmt.Errorf("provider %q is unavailable; install its CLI or use secrets:<key>", provider)
	case errors.Is(err, secrets.ErrUnsupportedProvider):
		return fmt.Errorf("provider %q is unsupported", provider)
	default:
		return fmt.Errorf("failed to store secret: %w", err)
	}
}

func normalizeListArgs(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	var items []string
	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			items = append(items, part)
		}
	}
	return dedupeStrings(items)
}

func dedupeStrings(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func selectEditor() string {
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor
	}
	if editor := strings.TrimSpace(os.Getenv("VISUAL")); editor != "" {
		return editor
	}
	return "vi"
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func validateSchedule(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	return len(strings.Fields(value)) == 5
}

func setLabel(
	configPath string,
	labelKey string,
	name string,
	keywords []string,
	schedule *string,
	languages []string,
	recipients []string,
	weight *float64,
) int {
	config, ok := loadYAMLConfig(configPath)
	if !ok {
		return 1
	}

	labelsValue := config[labelKey]
	var labels []any
	if labelsValue == nil {
		labels = []any{}
	} else {
		switch typed := labelsValue.(type) {
		case []any:
			labels = typed
		default:
			fmt.Fprintf(os.Stderr, "%s must be a list in config.yaml.\n", labelKey)
			return 1
		}
	}

	globalLanguages := []string{}
	if globalSection, ok := coerceStringMap(config["global"]); ok {
		globalLanguages = normalizeListValue(globalSection["languages"])
	}

	emailRecipients := []string{}
	if emailSection, ok := coerceStringMap(config["email"]); ok {
		emailRecipients = normalizeListValue(emailSection["to"])
	}

	existingIndex := -1
	existingEntry := map[string]any{}
	for idx, item := range labels {
		entry, ok := coerceStringMap(item)
		if !ok {
			continue
		}
		if normalizeString(entry["name"]) == name {
			existingIndex = idx
			existingEntry = entry
			break
		}
	}

	defaultLanguages := []string{}
	defaultRecipients := []string{}
	if len(existingEntry) > 0 {
		if langs := normalizeListValue(existingEntry["language"]); len(langs) > 0 {
			defaultLanguages = langs
		} else {
			defaultLanguages = normalizeListValue(existingEntry["languages"])
		}
		defaultRecipients = normalizeListValue(existingEntry["recipients"])
	} else {
		defaultLanguages = globalLanguages
		defaultRecipients = emailRecipients
	}

	if len(recipients) == 0 && len(defaultRecipients) == 0 {
		fmt.Fprintln(os.Stderr, "recipients are required when email.to is empty in config.yaml.")
		return 1
	}

	updated := cloneStringMap(existingEntry)
	updated["name"] = name
	updated["keywords"] = keywords
	if schedule != nil {
		updated["schedule"] = *schedule
	}
	if len(languages) > 0 {
		updated["language"] = languages
	} else if len(defaultLanguages) > 0 {
		updated["language"] = defaultLanguages
	}
	if len(recipients) > 0 {
		updated["recipients"] = recipients
	} else if len(defaultRecipients) > 0 {
		updated["recipients"] = defaultRecipients
	}
	if weight != nil {
		updated["weight"] = *weight
	} else if _, ok := updated["weight"]; !ok {
		updated["weight"] = 1.0
	}

	action := "updated"
	if existingIndex == -1 {
		labels = append(labels, updated)
		action = "added"
	} else {
		labels[existingIndex] = updated
	}

	config[labelKey] = labels
	if err := writeYAMLConfig(configPath, config); err != nil {
		return 1
	}

	if len(labelKey) > 0 {
		fmt.Printf("%s %s: %s\n", labelKey[:len(labelKey)-1], action, name)
		return 0
	}

	fmt.Printf("%s: %s\n", action, name)
	return 0
}

func loadYAMLConfig(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Config file not found: %s. Run `morningweave init` first.\n", path)
			return nil, false
		}
		fmt.Fprintf(os.Stderr, "Failed to read config file: %v\n", err)
		return nil, false
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid YAML in config: %v\n", err)
		return nil, false
	}

	if raw == nil {
		return map[string]any{}, true
	}

	config, ok := coerceStringMap(raw)
	if !ok {
		fmt.Fprintln(os.Stderr, "Config root must be a mapping/object.")
		return nil, false
	}
	return config, true
}

func writeYAMLConfig(path string, payload map[string]any) error {
	output, err := yaml.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config file: %v\n", err)
		return err
	}
	if err := os.WriteFile(path, output, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config file: %v\n", err)
		return err
	}
	return nil
}

func normalizeListValue(value any) []string {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				items = append(items, str)
			}
		}
		return dedupeStrings(items)
	case []string:
		return dedupeStrings(typed)
	case string:
		if strings.TrimSpace(typed) == "" {
			return []string{}
		}
		return dedupeStrings([]string{typed})
	default:
		return []string{}
	}
}

func normalizeString(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return ""
}

func coerceStringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, val := range typed {
			keyStr, ok := key.(string)
			if !ok {
				continue
			}
			converted[keyStr] = val
		}
		return converted, true
	default:
		return nil, false
	}
}

func coerceStringStringMap(value any) (map[string]string, bool) {
	switch typed := value.(type) {
	case map[string]string:
		return typed, true
	case map[string]any:
		converted := make(map[string]string, len(typed))
		for key, val := range typed {
			if str, ok := val.(string); ok {
				converted[key] = str
			}
		}
		return converted, true
	case map[any]any:
		converted := make(map[string]string, len(typed))
		for key, val := range typed {
			keyStr, ok := key.(string)
			if !ok {
				continue
			}
			if str, ok := val.(string); ok {
				converted[keyStr] = str
			}
		}
		return converted, true
	default:
		return nil, false
	}
}

func getSecretsValues(config map[string]any) (map[string]string, bool) {
	if config == nil {
		return nil, false
	}
	section, ok := coerceStringMap(config["secrets"])
	if !ok {
		if config["secrets"] == nil {
			return nil, true
		}
		fmt.Fprintln(os.Stderr, "secrets must be a mapping/object in config.yaml.")
		return nil, false
	}
	values, ok := coerceStringStringMap(section["values"])
	if !ok {
		if section["values"] == nil {
			return nil, true
		}
		fmt.Fprintln(os.Stderr, "secrets.values must be a mapping of strings in config.yaml.")
		return nil, false
	}
	return values, true
}

func ensureSecretsValues(config map[string]any) (map[string]string, bool) {
	if config == nil {
		return nil, false
	}
	section, ok := coerceStringMap(config["secrets"])
	if !ok {
		if config["secrets"] == nil {
			section = map[string]any{}
			config["secrets"] = section
		} else {
			fmt.Fprintln(os.Stderr, "secrets must be a mapping/object in config.yaml.")
			return nil, false
		}
	}
	values, ok := coerceStringStringMap(section["values"])
	if !ok {
		if section["values"] == nil {
			values = map[string]string{}
		} else {
			fmt.Fprintln(os.Stderr, "secrets.values must be a mapping of strings in config.yaml.")
			return nil, false
		}
	}
	section["values"] = values
	return values, true
}

func cloneStringMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, val := range source {
		clone[key] = val
	}
	return clone
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

type cronEntry struct {
	Schedule string
	Command  string
	Comment  string
}

func buildCronEntries(cfg config.Config, configPath string, command string) ([]cronEntry, []string) {
	entries := []cronEntry{}
	warnings := []string{}

	baseCommand := strings.TrimSpace(command)
	if baseCommand == "" {
		baseCommand = "morningweave"
	}

	addEntry := func(scopeLabel string, scheduleValue string, args []string) {
		trimmed := strings.TrimSpace(scheduleValue)
		if trimmed == "" {
			warnings = append(warnings, fmt.Sprintf("missing schedule for %s", scopeLabel))
			return
		}
		if _, err := schedule.Parse(trimmed); err != nil {
			warnings = append(warnings, fmt.Sprintf("invalid schedule for %s (%s): %v", scopeLabel, trimmed, err))
			return
		}

		fullArgs := append([]string{baseCommand}, args...)
		quoted := make([]string, 0, len(fullArgs))
		for _, arg := range fullArgs {
			quoted = append(quoted, shellQuote(arg))
		}
		entries = append(entries, cronEntry{
			Schedule: trimmed,
			Command:  strings.Join(quoted, " "),
			Comment:  scopeLabel,
		})
	}

	globalSchedule := strings.TrimSpace(cfg.Global.DefaultSchedule)
	addEntry("Global digest", globalSchedule, []string{"run", "--config", configPath})

	for _, tag := range cfg.Tags {
		scheduleValue := strings.TrimSpace(tag.Schedule)
		if scheduleValue == "" {
			scheduleValue = globalSchedule
		}
		label := fmt.Sprintf("Tag: %s", tag.Name)
		addEntry(label, scheduleValue, []string{"run", "--config", configPath, "--tag", tag.Name})
	}

	for _, category := range cfg.Categories {
		scheduleValue := strings.TrimSpace(category.Schedule)
		if scheduleValue == "" {
			scheduleValue = globalSchedule
		}
		label := fmt.Sprintf("Category: %s", category.Name)
		addEntry(label, scheduleValue, []string{"run", "--config", configPath, "--category", category.Name})
	}

	return entries, warnings
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func sampleDigestItems(now time.Time) []dedupe.MergedItem {
	return []dedupe.MergedItem{
		{
			Item: connectors.Item{
				Title:     "MorningWeave Test Item: Product Updates",
				URL:       "https://example.com/morningweave/test-item-1",
				Text:      "A short test summary for verifying your email delivery configuration and template rendering.",
				Timestamp: now,
				Source: connectors.SourceRef{
					Platform:   "hn",
					SourceType: "lists",
					Identifier: "top",
				},
			},
		},
		{
			Item: connectors.Item{
				Title:     "MorningWeave Test Item: Industry News",
				URL:       "https://example.com/morningweave/test-item-2",
				Text:      "Another test item to confirm multi-item digests show correctly across devices.",
				Timestamp: now,
				Source: connectors.SourceRef{
					Platform:   "reddit",
					SourceType: "subreddits",
					Identifier: "golang",
				},
			},
		},
	}
}
