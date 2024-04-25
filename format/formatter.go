package format

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"git.numtide.com/numtide/treefmt/config"

	"github.com/charmbracelet/log"
	"github.com/gobwas/glob"
)

// ErrCommandNotFound is returned when the Command for a Formatter is not available.
var ErrCommandNotFound = errors.New("formatter command not found in PATH")

// Formatter represents a command which should be applied to a filesystem.
type Formatter struct {
	name   string
	config *config.Formatter

	log        *log.Logger
	executable string // path to the executable described by Command

	// internal compiled versions of Includes and Excludes.
	includes []glob.Glob
	excludes []glob.Glob

	batch []string
}

// Executable returns the path to the executable defined by Command
func (f *Formatter) Executable() string {
	return f.executable
}

func (f *Formatter) Apply(ctx context.Context, paths []string, filter bool) error {
	// construct args, starting with config
	args := f.config.Options

	// If filter is true it indicates we are executing as part of a pipeline.
	// In such a scenario each formatter must sub filter the paths provided as different formatters might want different
	// files in a pipeline.
	if filter {
		// reset the batch
		f.batch = f.batch[:]

		// filter paths
		for _, path := range paths {
			if f.Wants(path) {
				f.batch = append(f.batch, path)
			}
		}

		// exit early if nothing to process
		if len(f.batch) == 0 {
			return nil
		}

		// append paths to the args
		args = append(args, f.batch...)
	} else {
		// exit early if nothing to process
		if len(paths) == 0 {
			return nil
		}

		// append paths to the args
		args = append(args, paths...)
	}

	// execute the command
	start := time.Now()
	cmd := exec.CommandContext(ctx, f.config.Command, args...)

	if out, err := cmd.CombinedOutput(); err != nil {
		f.log.Debugf("\n%v", string(out))
		// todo log output
		return err
	}

	f.log.Infof("%v files processed in %v", len(paths), time.Now().Sub(start))

	return nil
}

// Wants is used to test if a Formatter wants path based on it's configured Includes and Excludes patterns.
// Returns true if the Formatter should be applied to path, false otherwise.
func (f *Formatter) Wants(path string) bool {
	match := !PathMatches(path, f.excludes) && PathMatches(path, f.includes)
	if match {
		f.log.Debugf("match: %v", path)
	}
	return match
}

// NewFormatter is used to create a new Formatter.
func NewFormatter(
	name string,
	config *config.Formatter,
	globalExcludes []glob.Glob,
) (*Formatter, error) {
	var err error

	f := Formatter{}

	// capture config and the formatter's name
	f.name = name
	f.config = config

	// test if the formatter is available
	executable, err := exec.LookPath(config.Command)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrCommandNotFound
	} else if err != nil {
		return nil, err
	}
	f.executable = executable

	// initialise internal state
	if config.Pipeline == "" {
		f.log = log.WithPrefix(fmt.Sprintf("format | %s", name))
	} else {
		f.log = log.WithPrefix(fmt.Sprintf("format | %s[%s]", config.Pipeline, name))
	}

	f.includes, err = CompileGlobs(config.Includes)
	if err != nil {
		return nil, fmt.Errorf("%w: formatter '%v' includes", err, f.name)
	}

	f.excludes, err = CompileGlobs(config.Excludes)
	if err != nil {
		return nil, fmt.Errorf("%w: formatter '%v' excludes", err, f.name)
	}
	f.excludes = append(f.excludes, globalExcludes...)

	return &f, nil
}
