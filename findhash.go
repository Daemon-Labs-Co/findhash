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
	var targetSize int64 = -1   // -1 means unknown: no size pre-filter
	var refInfo os.FileInfo     // non-nil in reference-file mode; used to skip the file itself
	refName := ""               // reference filename, empty in -hash mode
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
		refInfo = info
		refName = ref
		if len(args) >= 2 {
			searchRoot = args[1]
		}
	}

	var matches []string
	_ = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if !d.Type().IsRegular() {
			return nil
		}
		// Don't report the reference file as a match against itself.
		if refInfo != nil {
			if ci, err := d.Info(); err == nil && os.SameFile(refInfo, ci) {
				return nil
			}
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
			matches = append(matches, path)
		}
		return nil
	})

	// Lead-in describes what we searched for and where.
	var lead string
	if refName != "" {
		lead = fmt.Sprintf("%s has a hash of %s, searching directory %s", refName, targetHash, searchRoot)
	} else {
		lead = fmt.Sprintf("searching directory %s for hash %s", searchRoot, targetHash)
	}

	if len(matches) == 0 {
		fmt.Printf("%s, no files found.\n", lead)
		os.Exit(1)
	}

	fmt.Printf("%s, located a copy here:\n%s\n", lead, strings.Join(matches, ",\n"))
	os.Exit(0)
}
