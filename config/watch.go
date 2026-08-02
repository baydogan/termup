package config

import (
	"context"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func Watch(ctx context.Context, path string, onReload func(*Config), onError func(error)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	target := filepath.Clean(path)
	if err := w.Add(filepath.Dir(target)); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(ev.Name) != target {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue // chmod/remove/rename yoksay
			}
			cfg, err := Load(path)
			if err != nil {
				onError(err)
				continue
			}
			onReload(cfg)
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			onError(err)
		}
	}
}
