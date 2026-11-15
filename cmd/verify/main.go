// Verify is the single entry point for formatting, source rules, vet and tests.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func main() {
	coverage := flag.Bool("coverage", false, "collect coverage and check its floor")
	integration := flag.Bool("integration", true, "test against TURBORELD as well")
	flag.Parse()
	if err := verify(*coverage, *integration); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) // R-ERR-1: a command reports its outcome through its exit status.
	}
}

func verify(coverage, integration bool) error {
	if err := sourceChecks(); err != nil {
		return err
	}
	if err := run("go", "vet", "./..."); err != nil {
		return err
	}
	if err := run("golangci-lint", "run", "./..."); err != nil {
		return err
	}
	args := []string{"test", "-race", "-count=1", "-timeout=3m"}
	if integration {
		args = append(args, "-tags=integration")
	}
	if coverage {
		args = append(args, "-coverprofile=coverage.out", "-covermode=atomic")
	}
	args = append(args, "./...")
	if err := run("go", args...); err != nil {
		return err
	}
	if coverage {
		return checkCoverage("coverage.out")
	}
	return nil
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}
