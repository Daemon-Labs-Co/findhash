// findhash - find any file whose sha256 content hash matches a target,
// searching recursively from a root (default ../../..).
//
// Usage:
//   findhash <reference-file> [search-root]   // hashes the reference file for you
//   findhash -hash <sha256>   [search-root]   // you supply the hash directly
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func main() {
	hashFlag := flag.String("hash", "", "target sha256 hash (instead of a reference file)")
	flag.Usage = func() {
		prog := filepath.Base(os.Args[0])
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s <reference-file> [search-root]\n", prog)
		fmt.Fprintf(os.Stderr, "  %s -hash <sha256>   [search-root]\n", prog)
		fmt.Fprintf(os.Stderr, "Default search-root: ../../..\n")
	}
	flag.Parse()
	args := flag.Args()

	var targetHash string
	var targetSize int64 = -1 // -1 means unknown: no size pre-filter
	searchRoot := "../../.."

	if *hashFlag != "" {
		targetHash = strings.ToLower(strings.TrimSpace(*hashFlag))
		if len(args) >= 1 {
			searchRoot = args[0]
		}
	} else {
		if len(args) < 1 {
			flag.Usage()
			os.Exit(2)
		}
		ref := args[0]
		info, err := os.Stat(ref)
		if err != nil || info.IsDir() {
			fmt.Fprintf(os.Stderr, "Not a file: %s\n", ref)
			os.Exit(1)
		}
		h, err := hashFile(ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hashing %s: %v\n", ref, err)
			os.Exit(1)
		}
		targetHash = h
		targetSize = info.Size()
		if len(args) >= 2 {
			searchRoot = args[1]
		}
	}

	found := false
	_ = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if !d.Type().IsRegular() {
			return nil
		}
		// Cheap size pre-filter before opening/hashing anything.
		if targetSize >= 0 {
			info, err := d.Info()
			if err != nil || info.Size() != targetSize {
				return nil
			}
		}
		h, err := hashFile(path)
		if err != nil {
			return nil
		}
		if h == targetHash {
			fmt.Printf("MATCH: %s\n", path)
			found = true
		}
		return nil
	})

	if found {
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "No file matching %s found under %s\n", targetHash, searchRoot)
	os.Exit(1)
}
