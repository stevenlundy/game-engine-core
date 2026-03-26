// cmd/glogtool is a CLI utility for inspecting and dumping .glog replay files
// produced by the game-engine-core replay log system.
//
// Usage:
//
//	glogtool inspect <path>   – print the session metadata header
//	glogtool dump    <path>   – pretty-print all step entries
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/game-engine/game-engine-core/pkg/engine"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}
	subcommand := os.Args[1]
	path := os.Args[2]

	var err error
	switch subcommand {
	case "inspect":
		err = cmdInspect(path)
	case "dump":
		err = cmdDump(path)
	default:
		fmt.Fprintf(os.Stderr, "glogtool: unknown subcommand %q\n\n", subcommand)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "glogtool %s: %v\n", subcommand, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `glogtool — inspect and dump .glog replay files

Usage:
  glogtool inspect <path>   Print the session metadata header
  glogtool dump    <path>   Pretty-print all step entries

`)
}

// cmdInspect opens the .glog at path and prints the session metadata as
// indented JSON to stdout.
func cmdInspect(path string) error {
	rr, err := engine.OpenReplayLog(path)
	if err != nil {
		return fmt.Errorf("open replay log: %w", err)
	}
	defer func() { _ = rr.Close() }()

	meta, err := rr.ReadMetadata()
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	out, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	fmt.Printf("%s\n", out)
	return nil
}

// cmdDump opens the .glog at path and pretty-prints every step entry to
// stdout, one JSON object per line (with indentation for readability).
func cmdDump(path string) error {
	rr, err := engine.OpenReplayLog(path)
	if err != nil {
		return fmt.Errorf("open replay log: %w", err)
	}
	defer func() { _ = rr.Close() }()

	// Consume and discard the metadata header.
	meta, err := rr.ReadMetadata()
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	// Print a brief header line.
	fmt.Printf("# session_id=%s  ruleset=%s  mode=%s  players=%v\n\n",
		meta.SessionID, meta.RulesetVersion, meta.Mode, meta.PlayerIDs)

	count := 0
	for {
		entry, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("entry %d: %w", count, err)
		}

		out, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal entry %d: %w", count, err)
		}
		fmt.Printf("%s\n", out)
		count++
	}

	fmt.Printf("\n# total entries: %d\n", count)
	return nil
}
