package secrets

import (
	"context"
	"errors"
	"os/exec"
)

var execCommandContext = exec.CommandContext

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := execCommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func isExecNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return errors.Is(execErr.Err, exec.ErrNotFound)
	}
	return errors.Is(err, exec.ErrNotFound)
}
