package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"morningweave/internal/config"
	"morningweave/internal/runner"
	"morningweave/internal/scaffold"
	"morningweave/internal/schedule"
	"morningweave/internal/storage"
)

const (
	defaultSinceWindow  = 24 * time.Hour
	defaultPollInterval = time.Minute
)

// Options controls the scheduler loop.
type Options struct {
	ConfigPath   string
	Logger       io.Writer
	PollInterval time.Duration
	Control      ControlPaths
}

// DueRun describes a pending scheduled run.
type DueRun struct {
	Scope    runner.RunScope
	Schedule string
	Since    time.Time
	Next     time.Time
}

// Plan captures due runs and the next scheduled time.
type Plan struct {
	Due  []DueRun
	Next time.Time
}

// RunLoop starts the scheduler loop and runs until context cancellation or stop signal.
func RunLoop(ctx context.Context, opts Options) error {
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		return fmt.Errorf("config path is required")
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	logf := func(format string, args ...any) {
		if opts.Logger == nil {
			return
		}
		now := time.Now().Format(time.RFC3339)
		fmt.Fprintf(opts.Logger, "[%s] "+format+"\n", append([]any{now}, args...)...)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	storagePath := strings.TrimSpace(cfg.Storage.Path)
	if storagePath == "" {
		storagePath = scaffold.DefaultStoragePath
	}
	control := opts.Control
	if control.PID == "" && control.Stop == "" && control.Log == "" {
		control = PathsForStorage(storagePath)
	}

	ClearStop(control.Stop)
	if err := WritePID(control.PID); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer RemovePID(control.PID)

	logf("scheduler started")

	for {
		if ctx.Err() != nil {
			logf("scheduler stopping (context)")
			return ctx.Err()
		}
		if StopRequested(control.Stop) {
			logf("scheduler stopping (stop file)")
			return nil
		}

		cfg, err = config.Load(configPath)
		if err != nil {
			logf("config load failed: %v", err)
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				return err
			}
			continue
		}

		storagePath = strings.TrimSpace(cfg.Storage.Path)
		if storagePath == "" {
			storagePath = scaffold.DefaultStoragePath
		}

		if _, err := storage.EnsureDatabase(storagePath); err != nil {
			logf("storage ensure failed: %v", err)
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				return err
			}
			continue
		}

		db, err := storage.Open(storagePath)
		if err != nil {
			logf("storage open failed: %v", err)
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				return err
			}
			continue
		}

		now := time.Now()
		plan, warnings, err := BuildPlan(cfg, db, now)
		_ = db.Close()
		if err != nil {
			logf("plan build failed: %v", err)
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				return err
			}
			continue
		}
		for _, warning := range warnings {
			logf("warning: %s", warning)
		}

		if len(plan.Due) == 0 {
			wait := pollInterval
			if !plan.Next.IsZero() {
				if until := time.Until(plan.Next); until > 0 && until < wait {
					wait = until
				}
			}
			if err := sleepWithContext(ctx, wait); err != nil {
				return err
			}
			continue
		}

		for _, due := range plan.Due {
			if ctx.Err() != nil {
				logf("scheduler stopping (context)")
				return ctx.Err()
			}
			if StopRequested(control.Stop) {
				logf("scheduler stopping (stop file)")
				return nil
			}
			runNow := time.Now()
			logf("running scope %s:%s (schedule %s)", due.Scope.Type, due.Scope.Name, due.Schedule)
			result, err := runner.RunOnce(ctx, cfg, runner.RunOptions{
				Scope: due.Scope,
				Since: due.Since,
				Until: runNow,
				Now:   runNow,
			})
			if err != nil {
				logf("run failed for %s:%s: %v", due.Scope.Type, due.Scope.Name, err)
				continue
			}

			if issues := runner.DetectAccessIssues(result.Warnings); len(issues) > 0 {
				if _, err := config.DisablePlatforms(configPath, issues); err != nil {
					logf("failed to disable platforms: %v", err)
				}
				for platform, reason := range issues {
					if strings.TrimSpace(reason) == "" {
						reason = "access unavailable"
					}
					logf("disabled %s due to access issue (%s)", platform, reason)
				}
			}
		}
	}
}

// BuildPlan computes due runs and the next scheduled time.
func BuildPlan(cfg config.Config, db *sql.DB, now time.Time) (Plan, []string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	entries := collectSchedules(cfg)
	warnings := []string{}
	plan := Plan{}

	for _, entry := range entries {
		scheduleValue := strings.TrimSpace(entry.Schedule)
		if scheduleValue == "" {
			warnings = append(warnings, fmt.Sprintf("missing schedule for %s:%s", entry.Scope.Type, entry.Scope.Name))
			continue
		}

		spec, err := schedule.Parse(scheduleValue)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("invalid schedule for %s:%s (%s): %v", entry.Scope.Type, entry.Scope.Name, scheduleValue, err))
			continue
		}

		lastRun, found, err := storage.GetLastRunForScope(db, entry.Scope.Type, entry.Scope.Name)
		if err != nil {
			return plan, warnings, err
		}

		from := now.Add(-defaultSinceWindow)
		if found && !lastRun.StartedAt.IsZero() {
			from = lastRun.StartedAt
		}

		next, err := spec.Next(from)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("schedule lookup failed for %s:%s: %v", entry.Scope.Type, entry.Scope.Name, err))
			continue
		}

		if !next.After(now) {
			since := now.Add(-defaultSinceWindow)
			if found && !lastRun.StartedAt.IsZero() {
				since = lastRun.StartedAt
			}
			plan.Due = append(plan.Due, DueRun{
				Scope:    entry.Scope,
				Schedule: scheduleValue,
				Since:    since,
				Next:     next,
			})
			continue
		}

		if plan.Next.IsZero() || next.Before(plan.Next) {
			plan.Next = next
		}
	}

	return plan, warnings, nil
}

type scheduleEntry struct {
	Scope    runner.RunScope
	Schedule string
}

func collectSchedules(cfg config.Config) []scheduleEntry {
	entries := []scheduleEntry{}

	globalSchedule := strings.TrimSpace(cfg.Global.DefaultSchedule)
	entries = append(entries, scheduleEntry{
		Scope:    runner.RunScope{Type: "global"},
		Schedule: globalSchedule,
	})

	for _, tag := range cfg.Tags {
		scheduleValue := strings.TrimSpace(tag.Schedule)
		if scheduleValue == "" {
			scheduleValue = globalSchedule
		}
		entries = append(entries, scheduleEntry{
			Scope: runner.RunScope{
				Type:       "tag",
				Name:       tag.Name,
				Keywords:   tag.Keywords,
				Languages:  tag.Language,
				Weight:     tag.Weight,
				Recipients: tag.Recipients,
			},
			Schedule: scheduleValue,
		})
	}

	for _, category := range cfg.Categories {
		scheduleValue := strings.TrimSpace(category.Schedule)
		if scheduleValue == "" {
			scheduleValue = globalSchedule
		}
		entries = append(entries, scheduleEntry{
			Scope: runner.RunScope{
				Type:       "category",
				Name:       category.Name,
				Keywords:   category.Keywords,
				Languages:  category.Language,
				Weight:     category.Weight,
				Recipients: category.Recipients,
			},
			Schedule: scheduleValue,
		})
	}

	return entries
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
