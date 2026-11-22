//go:build !windows

// Command installer is TeleCyborg's Windows setup program. It only runs on Windows.
package main

import "fmt"

func main() { fmt.Println("TeleCyborg setup runs on Windows only. Use build.sh to build it.") }
