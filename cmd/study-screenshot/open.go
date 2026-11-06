package main

import "os/exec"

func execLook(bin, arg string) error {
	return exec.Command(bin, arg).Start()
}
