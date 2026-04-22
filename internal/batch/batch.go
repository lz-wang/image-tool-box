package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

type Options struct {
	InputDir     string
	OutputDir    string
	Glob         string
	Recursive    bool
	Workers      int
	SkipExisting bool
	FailFast     bool
}

type FileError struct {
	Path string
	Err  error
}

type Result struct {
	Success int
	Skipped int
	Failed  int
	Errors  []FileError
}

type Processor func(inputPath, outputPath string) error
type OutputPathFunc func(relPath string) string

func Process(opts Options, outputPath OutputPathFunc, processor Processor) (Result, error) {
	files, err := DiscoverFiles(opts)
	if err != nil {
		return Result{}, err
	}
	if outputPath == nil {
		return Result{}, fmt.Errorf("output path function is required")
	}
	if processor == nil {
		return Result{}, fmt.Errorf("processor function is required")
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = min(4, max(1, runtime.NumCPU()))
	}

	type task struct {
		rel string
	}

	var (
		mu      sync.Mutex
		result  Result
		stopped atomic.Bool
		tasks   = make(chan task)
		wg      sync.WaitGroup
	)

	worker := func() {
		defer wg.Done()
		for t := range tasks {
			if opts.FailFast && stopped.Load() {
				continue
			}

			inputPath := filepath.Join(opts.InputDir, t.rel)
			outputRel := outputPath(t.rel)
			outputFile := filepath.Join(opts.OutputDir, outputRel)

			if opts.SkipExisting {
				if _, err := os.Stat(outputFile); err == nil {
					mu.Lock()
					result.Skipped++
					mu.Unlock()
					continue
				}
			}

			if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
				mu.Lock()
				result.Failed++
				result.Errors = append(result.Errors, FileError{Path: t.rel, Err: err})
				mu.Unlock()
				stopped.Store(opts.FailFast)
				continue
			}

			if err := processor(inputPath, outputFile); err != nil {
				mu.Lock()
				result.Failed++
				result.Errors = append(result.Errors, FileError{Path: t.rel, Err: err})
				mu.Unlock()
				stopped.Store(opts.FailFast)
				continue
			}

			mu.Lock()
			result.Success++
			mu.Unlock()
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	for _, rel := range files {
		if opts.FailFast && stopped.Load() {
			break
		}
		tasks <- task{rel: rel}
	}
	close(tasks)
	wg.Wait()

	if result.Failed > 0 {
		return result, fmt.Errorf("batch completed with %d failures", result.Failed)
	}
	return result, nil
}

func DiscoverFiles(opts Options) ([]string, error) {
	if opts.InputDir == "" {
		return nil, fmt.Errorf("input directory is required")
	}
	glob := opts.Glob
	if strings.TrimSpace(glob) == "" {
		glob = "*"
	}

	var files []string
	err := filepath.WalkDir(opts.InputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != opts.InputDir && !opts.Recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if !isImageFile(path) {
			return nil
		}

		rel, err := filepath.Rel(opts.InputDir, path)
		if err != nil {
			return err
		}
		if !matchesGlob(rel, glob) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func isImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func matchesGlob(relPath, glob string) bool {
	matchPath := filepath.ToSlash(relPath)
	if ok, err := filepath.Match(glob, matchPath); err == nil && ok {
		return true
	}
	if ok, err := filepath.Match(glob, filepath.Base(relPath)); err == nil && ok {
		return true
	}
	return false
}
