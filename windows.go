//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func GetTime() int64 {
	var t windows.Filetime
	windows.GetSystemTimePreciseAsFileTime(&t)
	return t.Nanoseconds()
}

func init() {
	stdout := windows.Handle(os.Stdout.Fd())
	var originalMode uint32

	windows.GetConsoleMode(stdout, &originalMode)
	windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

func install_to_appdata(self string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, `AppData\Local\Programs\Wire`)

	err = os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return err
	}

	path = filepath.Join(path, "wire.exe")

	err = copy_file(self, path)
	if err != nil {
		return err
	}

	return nil
}

func uninstall_from_appdata() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, `AppData\Local\Programs\Wire`)

	err = os.RemoveAll(path)
	if err != nil {
		return err
	}

	return nil
}

var RECV_PATH = `Software\Classes\directory\Background\shell\wire`
var FOLDER_PATH = `Software\Classes\directory\shell\wire`
var FILE_PATH = `Software\Classes\*\shell\wire`
var WIRE_PATH = `%USERPROFILE%\AppData\Local\Programs\Wire\wire.exe`
var ICON = `%SystemRoot%\System32\netcenter.dll,13`

func install_context_menu() error {
	//context menu for shift-right click in a folder (receive mode)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, RECV_PATH, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetStringValue("", "Wire Receive")
	k.SetStringValue("Extended", "")
	k.SetExpandStringValue("icon", ICON)
	k.Close()

	k, _, err = registry.CreateKey(registry.CURRENT_USER, RECV_PATH+`\command`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetExpandStringValue("", WIRE_PATH+` r "%V"`)
	k.Close()

	//context menu for shift-right click a folder (send the folder)
	k, _, err = registry.CreateKey(registry.CURRENT_USER, FOLDER_PATH, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetStringValue("", "Wire Send")
	k.SetStringValue("Extended", "")
	k.SetExpandStringValue("icon", ICON)
	k.Close()

	k, _, err = registry.CreateKey(registry.CURRENT_USER, FOLDER_PATH+`\command`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetExpandStringValue("", WIRE_PATH+` s "%V"`)
	k.Close()

	//context menu for shift-right click a file (send the file)
	k, _, err = registry.CreateKey(registry.CURRENT_USER, FILE_PATH, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetStringValue("", "Wire Send")
	k.SetStringValue("Extended", "")
	k.SetExpandStringValue("icon", ICON)
	k.Close()

	k, _, err = registry.CreateKey(registry.CURRENT_USER, FILE_PATH+`\command`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	k.SetExpandStringValue("", WIRE_PATH+` s "%1"`)
	k.Close()

	return nil
}

func uninstall_context_menu() error {
	//context menu for shift-right click in a folder (receive mode)
	k, err := registry.OpenKey(registry.CURRENT_USER, RECV_PATH+`\command`, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	k, err = registry.OpenKey(registry.CURRENT_USER, RECV_PATH, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	//context menu for shift-right click a folder (send the folder)
	k, err = registry.OpenKey(registry.CURRENT_USER, FOLDER_PATH+`\command`, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	k, err = registry.OpenKey(registry.CURRENT_USER, FOLDER_PATH, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	//context menu for shift-right click a file (send the file)
	k, err = registry.OpenKey(registry.CURRENT_USER, FILE_PATH+`\command`, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	k, err = registry.OpenKey(registry.CURRENT_USER, FILE_PATH, registry.ALL_ACCESS)
	if err == nil {
		registry.DeleteKey(k, "")
		k.Close()
	}

	return nil

}

func terminate() {
	//keep the CMD window open so errors can be read
	fmt.Println("Press enter to exit..")
	fmt.Scanln()
	os.Exit(1)
}

func install(self string) {
	if err := install_to_appdata(self); err != nil {
		fmt.Printf("failed to install: %s\n", err.Error())
		return
	}
	if err := install_context_menu(); err != nil {
		fmt.Printf("failed to configure registry: %s\n", err.Error())
		return
	}

	fmt.Println("wire installed")
}

func uninstall() {
	if err := uninstall_from_appdata(); err != nil {
		fmt.Printf("failed to uninstall: %s\n", err.Error())
		return
	}
	if err := uninstall_context_menu(); err != nil {
		fmt.Printf("failed to unconfigure registry: %s\n", err.Error())
		return
	}
	fmt.Println("wire uninstalled")
}
