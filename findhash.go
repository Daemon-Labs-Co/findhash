// findhash - find any file whose sha256 content hash matches a target,
// searching recursively from a root (default ../../..).
//
// Usage:
//   findhash <reference-file> [search-root]   // hashes the reference file for you
//   findhash -hash <sha256>   [search-root]   // you supply the hash directly
//
// Exit status (grep convention):
//   0  at least one copy found
//   1  no copies found
//   2  usage or runtime error (bad args, bad hash, unreadable root)
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

const (
	defaultRoot  = "../../.."
	sha256HexLen = 64
)

// hashFile returns the lowercase hex sha256 of a file's contents, streaming it
// so memory use stays flat regardless of file size.
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

func usage() {
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s <reference-file> [search-root]\n", prog)
	fmt.Fprintf(os.Stderr, "  %s -hash <sha256>   [search-root]\n", prog)
	fmt.Fprintf(os.Stderr, "Default search-root: %s\n", defaultRoot)
}

func run() int {
	hashFlag := flag.String("hash", "", "target sha256 hash to search for, instead of a reference file")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	var targetHash string
	var targetSize int64 = -1 // -1 means unknown: no size pre-filter
	var refInfo os.FileInfo   // non-nil in reference-file mode; used to skip the file itself
	var refName string        // reference filename, empty in -hash mode
	searchRoot := defaultRoot

	if *hashFlag != "" {
		h := strings.ToLower(strings.TrimSpace(*hashFlag))
		if len(h) != sha256HexLen {
			fmt.Fprintf(os.Stderr, "invalid hash %q: a sha256 is %d hex characters\n", *hashFlag, sha256HexLen)
			return 2
		}
		if _, err := hex.DecodeString(h); err != nil {
			fmt.Fprintf(os.Stderr, "invalid hash %q: not hexadecimal\n", *hashFlag)
			return 2
		}
		targetHash = h
		if len(args) > 1 {
			fmt.Fprintf(os.Stderr, "too many arguments: -hash mode takes only an optional search root\n")
			return 2
		}
		if len(args) == 1 {
			searchRoot = args[0]
		}
	} else {
		if len(args) < 1 {
			usage()
			return 2
		}
		if len(args) > 2 {
			fmt.Fprintf(os.Stderr, "too many arguments: expected <reference-file> [search-root]\n")
			return 2
		}
		ref := args[0]
		info, err := os.Stat(ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot read reference file %s: %v\n", ref, err)
			return 2
		}
		if info.IsDir() {
			fmt.Fprintf(os.Stderr, "reference must be a file, not a directory: %s\n", ref)
			return 2
		}
		h, err := hashFile(ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hashing reference file %s: %v\n", ref, err)
			return 2
		}
		targetHash = h
		targetSize = info.Size()
		refInfo = info
		refName = ref
		if len(args) == 2 {
			searchRoot = args[1]
		}
	}

	// Validate the root up front so an empty result means "no copies" rather
	// than "the directory you named doesn't exist".
	if rootInfo, err := os.Stat(searchRoot); err != nil {
		fmt.Fprintf(os.Stderr, "cannot access search root %s: %v\n", searchRoot, err)
		return 2
	} else if !rootInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "search root is not a directory: %s\n", searchRoot)
		return 2
	}

	var matches []string
	var skipped int

	// WalkDir visits entries in lexical order, so output is deterministic and
	// effectively sorted. It does not follow symlinks, which also rules out
	// directory cycles.
	_ = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			skipped++
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		// Only stat when we actually need to: to skip the reference file
		// itself, or to apply the size pre-filter.
		if refInfo != nil || targetSize >= 0 {
			info, err := d.Info()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
				skipped++
				return nil
			}
			// Identity by inode, so path spelling and hardlinks to the
			// reference don't register as false positives.
			if refInfo != nil && os.SameFile(refInfo, info) {
				return nil
			}
			// A different size cannot share the hash, so don't bother reading.
			if targetSize >= 0 && info.Size() != targetSize {
				return nil
			}
		}

		h, err := hashFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			skipped++
			return nil
		}
		if h == targetHash {
			matches = append(matches, path)
		}
		return nil
	})

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d entries were unreadable and skipped; results may be incomplete\n", skipped)
	}

	var lead string
	if refName != "" {
		lead = fmt.Sprintf("%s has a hash of %s, searching directory %s", refName, targetHash, searchRoot)
	} else {
		lead = fmt.Sprintf("searching directory %s for hash %s", searchRoot, targetHash)
	}

	if len(matches) == 0 {
		fmt.Printf("%s, no files found.\n", lead)
		return 1
	}
	fmt.Printf("%s, located a copy here:\n%s\n", lead, strings.Join(matches, ",\n"))
	return 0
}

func main() {
	os.Exit(run())
}
