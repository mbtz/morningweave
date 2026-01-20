package runner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"morningweave/internal/config"
	"morningweave/internal/connectors"
	"morningweave/internal/connectors/hn"
	instaconn "morningweave/internal/connectors/instagram"
	"morningweave/internal/connectors/reddit"
	xconn "morningweave/internal/connectors/x"
	"morningweave/internal/dedupe"
	"morningweave/internal/email"
	"morningweave/internal/ranking"
	"morningweave/internal/runlog"
	"morningweave/internal/scaffold"
	"morningweave/internal/secrets"
	"morningweave/internal/storage"
)

const (
	defaultSinceWindow    = 24 * time.Hour
	adaptiveMinRuns       = 10
	adaptiveMinMultiplier = 0.9
	adaptiveMaxMultiplier = 1.1
)

// RunScope captures the filter scope for a run.
type RunScope struct {
	Type       string
	Name       string
	Keywords   []string
	Languages  []string
	Weight     float64
	Recipients []string
}

// RunOptions customize a run invocation.
type RunOptions struct {
	Scope    RunScope
	WordCap  int
	MaxItems int
	Since    time.Time
	Until    time.Time
	Now      time.Time
}

// RunResult captures the run outputs.
type RunResult struct {
	Record   storage.RunRecord
	Digest   email.RenderResult
	Warnings []string
}

// RunOnce orchestrates a single run pipeline.
func RunOnce(ctx context.Context, cfg config.Config, opts RunOptions) (RunResult, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	since := opts.Since
	if since.IsZero() {
		since = now.Add(-defaultSinceWindow)
	}
	until := opts.Until
	if until.IsZero() {
		until = now
	}

	wordCap := opts.WordCap
	if wordCap <= 0 {
		wordCap = cfg.Global.Digest.WordCap
	}
	if wordCap <= 0 {
		wordCap = email.DefaultWordCap
	}

	maxItems := opts.MaxItems
	if maxItems <= 0 {
		maxItems = cfg.Global.Digest.MaxItems
	}
	if maxItems <= 0 {
		maxItems = email.DefaultMaxItems
	}

	storagePath := strings.TrimSpace(cfg.Storage.Path)
	if storagePath == "" {
		storagePath = scaffold.DefaultStoragePath
	}

	if _, err := storage.EnsureDatabase(storagePath); err != nil {
		return RunResult{}, fmt.Errorf("ensure storage: %w", err)
	}

	db, err := storage.Open(storagePath)
	if err != nil {
		return RunResult{}, fmt.Errorf("open storage: %w", err)
	}
	defer db.Close()

	scope := opts.Scope
	scopeType := strings.TrimSpace(scope.Type)
	scopeName := strings.TrimSpace(scope.Name)

	record := storage.RunRecord{
		Status:    "running",
		StartedAt: now,
		ScopeType: scopeType,
		ScopeName: scopeName,
	}
	record, err = storage.CreateRun(db, record)
	if err != nil {
		return RunResult{}, fmt.Errorf("create run: %w", err)
	}

	result := RunResult{Record: record}
	if adjusted, err := applyAdaptiveWeight(db, scope); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("adaptive weight: %v", err))
	} else {
		scope = adjusted
	}

	items, warnings, counts, err := fetchItems(ctx, cfg, scope, since, until)
	result.Warnings = append(result.Warnings, warnings...)
	if counts != nil {
		result.Record.PlatformCounts = counts
	}
	result.Record.ItemsFetched = len(items)
	if err != nil {
		result.Record.Status = "error"
		result.Record.Error = err.Error()
		result.Record.FinishedAt = time.Now()
		if updateErr := storage.UpdateRun(db, result.Record); updateErr != nil {
			return result, fmt.Errorf("%w (update run: %v)", err, updateErr)
		}
		writeRunLog(storagePath, cfg, &result)
		return result, err
	}

	merged := dedupe.Dedupe(items)
	ranked := rankMergedItems(merged, rankOptionsFor(cfg, scope, now))
	result.Record.ItemsRanked = len(ranked)

	ordered := make([]dedupe.MergedItem, 0, len(ranked))
	for _, item := range ranked {
		ordered = append(ordered, item.Merged)
	}

	subject := renderSubject(cfg.Email.Subject, now)
	if strings.TrimSpace(subject) == "" {
		subject = "MorningWeave Digest"
	}

	var renderResult email.RenderResult
	if len(ordered) > 0 {
		renderResult, err = email.RenderDigest(ordered, email.RenderOptions{
			Title:       subject,
			WordCap:     wordCap,
			MaxItems:    maxItems,
			GeneratedAt: now,
		})
		if err != nil {
			if errors.Is(err, email.ErrNoItems) {
				err = nil
			}
		}
	}

	if err != nil {
		result.Record.Status = "error"
		result.Record.Error = err.Error()
		result.Record.FinishedAt = time.Now()
		if updateErr := storage.UpdateRun(db, result.Record); updateErr != nil {
			return result, fmt.Errorf("%w (update run: %v)", err, updateErr)
		}
		writeRunLog(storagePath, cfg, &result)
		return result, err
	}

	result.Digest = renderResult
	result.Record.ItemsSent = renderResult.Items
	result.Record.EmailSent = false

	if renderResult.Items == 0 {
		result.Record.Status = "empty"
		result.Record.FinishedAt = time.Now()
		if updateErr := storage.UpdateRun(db, result.Record); updateErr != nil {
			return result, fmt.Errorf("update run: %w", updateErr)
		}
		if err := updateAdaptiveWeights(db, scope, result.Record.Status, result.Record.ItemsSent, now); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("adaptive weights: %v", err))
		}
		result.Warnings = append(result.Warnings, applyRetention(db, storagePath, cfg, now)...)
		writeRunLog(storagePath, cfg, &result)
		return result, nil
	}

	recipients := opts.Scope.Recipients
	if len(recipients) == 0 {
		recipients = cfg.Email.To
	}
	from := strings.TrimSpace(cfg.Email.From)
	if from == "" {
		err = errors.New("email.from is required")
	} else if len(recipients) == 0 {
		err = errors.New("email recipients are required")
	} else {
		resolver := secrets.NewResolver(cfg.Secrets.Values)
		var sendWarnings []string
		var sender email.Sender
		sender, sendWarnings, err = email.NewSenderFromConfig(cfg.Email, resolver)
		result.Warnings = append(result.Warnings, sendWarnings...)
		if err == nil {
			sendErr := sender.Send(ctx, email.Message{
				From:    from,
				To:      recipients,
				Subject: subject,
				HTML:    renderResult.HTML,
			})
			if sendErr != nil {
				err = sendErr
			} else {
				result.Record.EmailSent = true
				result.Record.Status = "success"
			}
		}
	}

	if err != nil {
		result.Record.Status = "error"
		result.Record.Error = err.Error()
		result.Record.FinishedAt = time.Now()
		if updateErr := storage.UpdateRun(db, result.Record); updateErr != nil {
			return result, fmt.Errorf("%w (update run: %v)", err, updateErr)
		}
		writeRunLog(storagePath, cfg, &result)
		return result, err
	}

	result.Record.FinishedAt = time.Now()
	if updateErr := storage.UpdateRun(db, result.Record); updateErr != nil {
		return result, fmt.Errorf("update run: %w", updateErr)
	}
	if err := updateAdaptiveWeights(db, scope, result.Record.Status, result.Record.ItemsSent, now); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("adaptive weights: %v", err))
	}

	result.Warnings = append(result.Warnings, applyRetention(db, storagePath, cfg, now)...)
	writeRunLog(storagePath, cfg, &result)

	return result, nil
}

func applyAdaptiveWeight(db *sql.DB, scope RunScope) (RunScope, error) {
	key, ok := adaptiveWeightKey(scope)
	if !ok || db == nil {
		return scope, nil
	}

	record, found, err := storage.GetTagWeight(db, key)
	if err != nil {
		return scope, err
	}
	if !found || record.Weight <= 0 {
		return scope, nil
	}

	base := scope.Weight
	if base <= 0 {
		base = 1.0
	}
	scope.Weight = base * record.Weight
	return scope, nil
}

func updateAdaptiveWeights(db *sql.DB, scope RunScope, status string, itemsSent int, now time.Time) error {
	key, ok := adaptiveWeightKey(scope)
	if !ok || db == nil {
		return nil
	}
	if status != "success" && status != "empty" {
		return nil
	}

	record, found, err := storage.GetTagWeight(db, key)
	if err != nil {
		return err
	}

	runs := 0
	hits := 0
	weight := 1.0
	if found {
		runs = record.Runs
		hits = record.Hits
		if record.Weight > 0 {
			weight = record.Weight
		}
	}

	runs++
	if itemsSent > 0 {
		hits++
	}

	if runs >= adaptiveMinRuns {
		successRate := float64(hits) / float64(runs)
		weight = adaptiveMinMultiplier + (adaptiveMaxMultiplier-adaptiveMinMultiplier)*successRate
	}

	return storage.UpsertTagWeights(db, []storage.TagWeightRecord{
		{
			TagName:   key,
			Weight:    weight,
			Runs:      runs,
			Hits:      hits,
			UpdatedAt: now,
		},
	})
}

func adaptiveWeightKey(scope RunScope) (string, bool) {
	name := strings.TrimSpace(scope.Name)
	if name == "" {
		return "", false
	}
	keyName := strings.ToLower(name)
	keyType := strings.ToLower(strings.TrimSpace(scope.Type))
	if keyType != "tag" && keyType != "category" {
		return "", false
	}
	return fmt.Sprintf("%s:%s", keyType, keyName), true
}

func fetchItems(ctx context.Context, cfg config.Config, scope RunScope, since time.Time, until time.Time) ([]connectors.Item, []string, map[string]int, error) {
	var items []connectors.Item
	warnings := []string{}
	counts := map[string]int{}
	resolver := secrets.NewResolver(cfg.Secrets.Values)

	if cfg.Platforms.HN != nil && cfg.Platforms.HN.Enabled {
		sources := buildSources(cfg.Platforms.HN)
		if len(sources) == 0 {
			warnings = append(warnings, "hn: no sources configured")
		} else {
			conn := hn.New()
			result, err := conn.Fetch(ctx, connectors.FetchRequest{
				Sources:  sources,
				Keywords: scope.Keywords,
				Since:    since,
				Until:    until,
			})
			if err != nil {
				return items, warnings, counts, fmt.Errorf("hn fetch: %w", err)
			}
			warnings = append(warnings, result.Warnings...)
			items = append(items, result.Items...)
			counts["hn"] = len(result.Items)
		}
	}

	if cfg.Platforms.Reddit != nil && cfg.Platforms.Reddit.Enabled {
		sources := buildSources(cfg.Platforms.Reddit)
		if len(sources) == 0 {
			warnings = append(warnings, "reddit: no sources configured")
		} else if strings.TrimSpace(cfg.Platforms.Reddit.CredentialsRef) == "" {
			warnings = append(warnings, "reddit: credentials_ref is required")
			warnings = AppendAuthRequirementHint(warnings, "reddit")
		} else {
			credsRaw, err := resolver.Resolve(cfg.Platforms.Reddit.CredentialsRef)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("reddit: resolve credentials: %v", err))
				warnings = AppendAuthRequirementHint(warnings, "reddit")
			} else {
				creds, err := reddit.ParseCredentials(credsRaw)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("reddit: parse credentials: %v", err))
					warnings = AppendAuthRequirementHint(warnings, "reddit")
				} else {
					conn := reddit.New(reddit.WithCredentials(creds))
					result, err := conn.Fetch(ctx, connectors.FetchRequest{
						Sources:  sources,
						Keywords: scope.Keywords,
						Since:    since,
						Until:    until,
					})
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("reddit: fetch failed: %v", err))
					} else {
						warnings = append(warnings, result.Warnings...)
						items = append(items, result.Items...)
						counts["reddit"] = len(result.Items)
					}
				}
			}
		}
	}
	if cfg.Platforms.X != nil && cfg.Platforms.X.Enabled {
		sources := buildSources(cfg.Platforms.X)
		if len(sources) == 0 {
			warnings = append(warnings, "x: no sources configured")
		} else if strings.TrimSpace(cfg.Platforms.X.CredentialsRef) == "" {
			warnings = append(warnings, "x: credentials_ref is required")
			warnings = AppendAuthRequirementHint(warnings, "x")
		} else {
			credsRaw, err := resolver.Resolve(cfg.Platforms.X.CredentialsRef)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("x: resolve credentials: %v", err))
				warnings = AppendAuthRequirementHint(warnings, "x")
			} else {
				creds, err := xconn.ParseCredentials(credsRaw)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("x: parse credentials: %v", err))
					warnings = AppendAuthRequirementHint(warnings, "x")
				} else {
					conn := xconn.New(xconn.WithCredentials(creds))
					result, err := conn.Fetch(ctx, connectors.FetchRequest{
						Sources:  sources,
						Keywords: scope.Keywords,
						Since:    since,
						Until:    until,
					})
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("x: fetch failed: %v", err))
					} else {
						warnings = append(warnings, result.Warnings...)
						items = append(items, result.Items...)
						counts["x"] = len(result.Items)
					}
				}
			}
		}
	}
	if cfg.Platforms.Instagram != nil && cfg.Platforms.Instagram.Enabled {
		sources := buildSources(cfg.Platforms.Instagram)
		if len(sources) == 0 {
			warnings = append(warnings, "instagram: no sources configured")
		} else if strings.TrimSpace(cfg.Platforms.Instagram.CredentialsRef) == "" {
			warnings = append(warnings, "instagram: credentials_ref is required")
			warnings = AppendAuthRequirementHint(warnings, "instagram")
		} else {
			credsRaw, err := resolver.Resolve(cfg.Platforms.Instagram.CredentialsRef)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("instagram: resolve credentials: %v", err))
				warnings = AppendAuthRequirementHint(warnings, "instagram")
			} else {
				creds, err := instaconn.ParseCredentials(credsRaw)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("instagram: parse credentials: %v", err))
					warnings = AppendAuthRequirementHint(warnings, "instagram")
				} else {
					conn := instaconn.New(instaconn.WithCredentials(creds))
					result, err := conn.Fetch(ctx, connectors.FetchRequest{
						Sources:  sources,
						Keywords: scope.Keywords,
						Since:    since,
						Until:    until,
					})
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("instagram: fetch failed: %v", err))
					} else {
						warnings = append(warnings, result.Warnings...)
						items = append(items, result.Items...)
						counts["instagram"] = len(result.Items)
					}
				}
			}
		}
	}

	if len(items) == 0 && len(warnings) == 0 {
		warnings = append(warnings, "no enabled platforms configured")
	}

	return items, warnings, counts, nil
}

func buildSources(cfg *config.PlatformConfig) []connectors.Source {
	if cfg == nil {
		return nil
	}
	var sources []connectors.Source
	for sourceType, identifiers := range cfg.Sources {
		for _, identifier := range identifiers {
			trimmed := strings.TrimSpace(identifier)
			if trimmed == "" {
				continue
			}
			sources = append(sources, connectors.Source{
				SourceType: sourceType,
				Identifier: trimmed,
				Weight:     lookupSourceWeight(cfg, sourceType, trimmed),
			})
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].SourceType == sources[j].SourceType {
			return sources[i].Identifier < sources[j].Identifier
		}
		return sources[i].SourceType < sources[j].SourceType
	})
	return sources
}

func lookupSourceWeight(cfg *config.PlatformConfig, sourceType string, identifier string) float64 {
	if cfg == nil {
		return 1.0
	}
	lookup, ok := cfg.SourceWeights[strings.TrimSpace(sourceType)]
	if !ok {
		return 1.0
	}
	weight, ok := lookup[strings.TrimSpace(identifier)]
	if !ok || weight <= 0 {
		return 1.0
	}
	return weight
}

func rankOptionsFor(cfg config.Config, scope RunScope, now time.Time) rankOptions {
	langs := scope.Languages
	if len(langs) == 0 {
		langs = cfg.Global.Languages
	}
	weight := scope.Weight
	if weight <= 0 {
		weight = 1.0
	}

	return rankOptions{
		Keywords:              scope.Keywords,
		AllowedLanguages:      langs,
		MinLanguageConfidence: 0.6,
		SourceWeight:          buildSourceWeightFn(cfg),
		Now:                   now,
		TagWeight:             weight,
	}
}

func buildSourceWeightFn(cfg config.Config) func(item connectors.Item) float64 {
	return func(item connectors.Item) float64 {
		platform := strings.ToLower(strings.TrimSpace(item.Source.Platform))
		var platformCfg *config.PlatformConfig
		switch platform {
		case "hn":
			platformCfg = cfg.Platforms.HN
		case "reddit":
			platformCfg = cfg.Platforms.Reddit
		case "x":
			platformCfg = cfg.Platforms.X
		case "instagram":
			platformCfg = cfg.Platforms.Instagram
		default:
			platformCfg = nil
		}

		platformWeight := 1.0
		if platformCfg != nil && platformCfg.Weight > 0 {
			platformWeight = platformCfg.Weight
		}

		sourceWeight := 1.0
		if platformCfg != nil {
			sourceWeight = lookupSourceWeight(platformCfg, item.Source.SourceType, item.Source.Identifier)
		}
		return platformWeight * sourceWeight
	}
}

func renderSubject(template string, now time.Time) string {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, "{{date}}", now.Format("2006-01-02"))
}

func applyRetention(db *sql.DB, storagePath string, cfg config.Config, now time.Time) []string {
	if db == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	var warnings []string

	if cfg.Logging.RetentionDays > 0 {
		cutoff := now.Add(-time.Duration(cfg.Logging.RetentionDays) * 24 * time.Hour)
		if _, err := storage.PruneRunsBefore(db, cutoff); err != nil {
			warnings = append(warnings, fmt.Sprintf("prune runs: %v", err))
		}
		if storagePath != "" {
			if _, err := runlog.Prune(storagePath, cutoff); err != nil {
				warnings = append(warnings, fmt.Sprintf("prune run logs: %v", err))
			}
		}
	}

	if cfg.Storage.SeenRetentionDays > 0 {
		cutoff := now.Add(-time.Duration(cfg.Storage.SeenRetentionDays) * 24 * time.Hour)
		if _, err := storage.PruneSeenItemsBefore(db, cutoff); err != nil {
			warnings = append(warnings, fmt.Sprintf("prune seen items: %v", err))
		}
		if _, err := storage.PruneDedupeMapBefore(db, cutoff); err != nil {
			warnings = append(warnings, fmt.Sprintf("prune dedupe map: %v", err))
		}
	}

	return warnings
}

func writeRunLog(storagePath string, cfg config.Config, result *RunResult) {
	if result == nil || strings.TrimSpace(storagePath) == "" {
		return
	}
	entry := runlog.Entry{
		ID:             result.Record.ID,
		StartedAt:      result.Record.StartedAt,
		FinishedAt:     result.Record.FinishedAt,
		Status:         result.Record.Status,
		ScopeType:      result.Record.ScopeType,
		ScopeName:      result.Record.ScopeName,
		ItemsFetched:   result.Record.ItemsFetched,
		ItemsRanked:    result.Record.ItemsRanked,
		ItemsSent:      result.Record.ItemsSent,
		EmailSent:      result.Record.EmailSent,
		PlatformCounts: result.Record.PlatformCounts,
		Error:          result.Record.Error,
		Warnings:       append([]string{}, result.Warnings...),
	}
	if err := runlog.Write(storagePath, entry, cfg.Secrets.Values); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("write run log: %v", err))
	}
}

type rankOptions struct {
	Keywords              []string
	AllowedLanguages      []string
	MinLanguageConfidence float64
	SourceWeight          func(item connectors.Item) float64
	Now                   time.Time
	TagWeight             float64
}

type scoredMergedItem struct {
	Merged             dedupe.MergedItem
	Score              float64
	Components         ranking.Components
	TagMatch           ranking.MatchResult
	Language           string
	LanguageConfidence float64
	SourceWeight       float64
}

func rankMergedItems(items []dedupe.MergedItem, opts rankOptions) []scoredMergedItem {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	minConfidence := opts.MinLanguageConfidence
	if minConfidence <= 0 {
		minConfidence = 0.6
	}
	weightFn := opts.SourceWeight
	if weightFn == nil {
		weightFn = func(connectors.Item) float64 { return 1.0 }
	}
	tagWeight := opts.TagWeight
	if tagWeight <= 0 {
		tagWeight = 1.0
	}
	allowed := normalizeLanguageSet(opts.AllowedLanguages)

	scored := make([]scoredMergedItem, 0, len(items))
	for _, merged := range items {
		item := merged.Item
		text := strings.TrimSpace(item.Title + " " + item.Text)
		lang, confidence := ranking.DetectLanguage(text)
		if len(allowed) > 0 {
			if _, ok := allowed[lang]; !ok || confidence < minConfidence {
				continue
			}
		}

		match := ranking.MatchKeywords(text, opts.Keywords)
		if len(opts.Keywords) > 0 && match.Score == 0 {
			continue
		}

		tagScore := match.Score * tagWeight
		sourceWeight := weightFn(item)
		components := ranking.ScoreComponents(item, tagScore, sourceWeight, now)
		score := ranking.CombinedScore(components)

		scored = append(scored, scoredMergedItem{
			Merged:             merged,
			Score:              score,
			Components:         components,
			TagMatch:           match,
			Language:           lang,
			LanguageConfidence: confidence,
			SourceWeight:       sourceWeight,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i]
		right := scored[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		leftTime := left.Merged.Item.Timestamp
		rightTime := right.Merged.Item.Timestamp
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return left.Merged.Item.Title < right.Merged.Item.Title
	})

	return scored
}

func normalizeLanguageSet(values []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result
}
