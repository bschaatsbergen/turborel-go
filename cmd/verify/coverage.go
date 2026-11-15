// Coverage floors are measured from executed statements and may only rise.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// This initial floor covers the public driver and protocol implementation.
const driverCoverageFloor = 70.0

func checkCoverage(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }() // The file is read-only.
	scanner := bufio.NewScanner(file)
	var total, covered uint64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || strings.Contains(line, "/cmd/") || strings.Contains(line, "/examples/") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return fmt.Errorf("invalid coverage record")
		}
		statements, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return err
		}
		hits, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return err
		}
		total += statements
		if hits > 0 {
			covered += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if total == 0 {
		return fmt.Errorf("coverage contains no driver statements")
	}
	percent := float64(covered) / float64(total) * 100
	fmt.Printf("driver coverage: %.1f%% (floor %.1f%%)\n", percent, driverCoverageFloor)
	if percent < driverCoverageFloor {
		return fmt.Errorf("R-COV-2: driver coverage is below its floor")
	}
	return nil
}
