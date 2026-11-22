//go:build !windows

package speech

import "os/exec"

func hideWindow(*exec.Cmd) {}
func bindToJob(*exec.Cmd)  {}
func needsVCRedist() bool  { return false }
