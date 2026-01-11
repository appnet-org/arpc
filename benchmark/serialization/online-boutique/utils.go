package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeTimings writes timing data (in nanoseconds) to a file, one value per line
func writeTimings(filename string, timings []int64) error {
	// Create subdirectory for profile data
	dir := "profile_data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to file in subdirectory
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, t := range timings {
		fmt.Fprintf(f, "%d\n", t)
	}
	return nil
}

// writeSizes writes size data (in bytes) to a file, one value per line
func writeSizes(filename string, sizes []int) error {
	// Create subdirectory for profile data
	dir := "profile_data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to file in subdirectory
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, s := range sizes {
		fmt.Fprintf(f, "%d\n", s)
	}
	return nil
}
