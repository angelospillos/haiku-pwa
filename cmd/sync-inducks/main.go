// Command sync-inducks downloads the daily Inducks data dump and builds the
// local search index that the server uses for Inducks lookups.
//
// Usage:
//
//	sync-inducks                  download the dump and rebuild the index
//	sync-inducks -archive f.tgz   use an archive that is already downloaded
//	sync-inducks -dir path/isv    use .isv files that are already extracted
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/config"
	"github.com/angelospillos/haiku-pwa/internal/inducks"
)

const (
	archiveURL = "https://inducks.org/inducks/isv.tgz"
	userAgent  = "haiku-pwa/0.1 (personal comic collection; +https://github.com/angelospillos/haiku-pwa)"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sync-inducks:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("sync-inducks", flag.ExitOnError)
	archive := flags.String("archive", "", "use this downloaded isv.tgz instead of downloading it")
	dir := flags.String("dir", "", "use already extracted .isv files from this directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	source := *dir
	if source == "" {
		workDir := filepath.Join(cfg.DataDir, "inducks-download")
		archivePath := *archive
		if archivePath == "" {
			archivePath = filepath.Join(workDir, "isv.tgz")
			if err := download(ctx, logger, archiveURL, archivePath); err != nil {
				return err
			}
		}
		logger.Info("extracting inducks tables", "archive", archivePath)
		if err := extract(archivePath, workDir); err != nil {
			return err
		}
		source = filepath.Join(workDir, "isv")
	}

	logger.Info("importing inducks data", "source", source)
	counts, err := inducks.Import(ctx, source, cfg.InducksDBPath)
	if err != nil {
		return err
	}
	logger.Info("inducks index built",
		"path", cfg.InducksDBPath,
		"issues", counts.Issues,
		"publications", counts.Publications,
		"alternative_names", counts.PublicationNames,
		"countries", counts.Countries,
	)
	logger.Info("restart the server so it picks up the new index")
	return nil
}

func download(ctx context.Context, logger *slog.Logger, url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, response.StatusCode)
	}

	logger.Info("downloading inducks dump", "url", url)
	partial := dest + ".part"
	file, err := os.Create(partial) //nolint:gosec // G304: the destination is chosen by the operator's configuration
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(partial)
		return fmt.Errorf("save inducks dump: %w", err)
	}
	if err := os.Rename(partial, dest); err != nil {
		return err
	}
	logger.Info("downloaded inducks dump", "megabytes", written/(1<<20), "path", dest)
	return nil
}

// extract copies the .isv members the index needs from the archive into
// workDir/isv. Only the fixed member names are written, so no path from the
// archive is ever used to create a file.
func extract(archivePath, workDir string) error {
	file, err := os.Open(archivePath) //nolint:gosec // G304: the archive is named by the operator's flag or download path
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", archivePath, err)
	}
	defer func() { _ = gzipReader.Close() }()

	wanted := make(map[string]bool, len(inducks.Members))
	for _, member := range inducks.Members {
		wanted["isv/"+member] = true
	}
	outDir := filepath.Join(workDir, "isv")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return err
	}

	found := 0
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", archivePath, err)
		}
		if header.Typeflag != tar.TypeReg || !wanted[header.Name] {
			continue
		}
		if err := writeMember(reader, filepath.Join(outDir, path.Base(header.Name))); err != nil {
			return err
		}
		found++
	}
	if found != len(inducks.Members) {
		return fmt.Errorf("%s is missing %d of the expected inducks files", archivePath, len(inducks.Members)-found)
	}
	return nil
}

func writeMember(src io.Reader, dest string) error {
	out, err := os.Create(dest) //nolint:gosec // G304: dest is always under the operator's data directory
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	return errors.Join(copyErr, out.Close())
}
