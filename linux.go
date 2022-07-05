//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

//assume this is in PATH, may not be
var LOCAL_BIN = ".local/bin"

func terminate() {
	os.Exit(1)
}

func install_to_local(self string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	file := filepath.Join(home, LOCAL_BIN+"/wire")

	if err = copy_file(self, file); err != nil {
		return err
	}

	return nil
}

func uninstall_from_local() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	file := filepath.Join(home, LOCAL_BIN+"/wire")

	if err = os.Remove(file); err != nil {
		return err
	}

	return nil
}

func install(self string) {
	if err := install_to_local(self); err != nil {
		fmt.Printf("failed to install: %s\n", err.Error())
		return
	}

	fmt.Println("wire installed")
}

func uninstall() {
	if err := uninstall_from_local(); err != nil {
		fmt.Printf("failed to uninstall: %s\n", err.Error())
		return
	}

	fmt.Println("wire uninstalled")
}
