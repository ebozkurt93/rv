package main

import "fmt"

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println("rv", version, "— TUI not implemented yet")
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println("rv", version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
