// reconcile-assets inventories legacy staged blobs that have no database row.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"vedam/lms/internal/runtime"
)

func main() {
	move := flag.Bool("quarantine", false, "move candidates to a private quarantine directory")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := reconcile(ctx, *move); err != nil {
		log.Fatal(err)
	}
}

func reconcile(ctx context.Context, move bool) error {
	cfg, err := runtime.LoadConfig()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(cfg.Assets)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Println("Asset storage does not exist. No files to reconcile.")
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat("submissions")
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Println("No submission directory. No files to reconcile.")
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("submissions must be a real directory")
	}
	dir, err := root.Open("submissions")
	if err != nil {
		return err
	}
	defer dir.Close()

	db, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("could not connect to the asset metadata database")
	}
	defer db.Close(ctx)
	// A standard install contains metadata in the active schema. Stop on a
	// missing or inaccessible table before making any filesystem changes.
	if _, err := db.Exec(ctx, "SELECT 1 FROM assets LIMIT 0"); err != nil {
		return fmt.Errorf("could not read the asset metadata table: %w", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	quarantine := ""
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			id, err := uuid.Parse(entry.Name())
			if err != nil || id.String() != entry.Name() {
				continue
			}
			key := path.Join("submissions", id.String())
			info, err := root.Lstat(key)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
				continue
			}
			var tracked bool
			if err := db.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM assets WHERE id=$1 OR storage_key=$2
			)`, id.String(), key).Scan(&tracked); err != nil {
				return err
			}
			if tracked {
				continue
			}
			if move {
				if quarantine == "" {
					quarantine = "submission-quarantine-" + uuid.NewString()
					if err := root.Mkdir(quarantine, 0700); err != nil {
						return err
					}
					fmt.Println("Quarantine directory", quarantine)
				}
				current, err := root.Lstat(key)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				if !os.SameFile(info, current) || !current.Mode().IsRegular() || !current.ModTime().Equal(info.ModTime()) {
					continue
				}
				if err := root.Rename(key, path.Join(quarantine, id.String())); err != nil {
					return err
				}
			}
			fmt.Println(key)
			count++
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if move {
		fmt.Printf("Quarantined %d untracked submission files.\n", count)
	} else {
		fmt.Printf("Found %d untracked submission files. No files moved.\n", count)
	}
	return nil
}
