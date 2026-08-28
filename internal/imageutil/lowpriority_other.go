//go:build !unix

package imageutil

import "os/exec"

// runEncodeCommand runs an artwork encode subprocess. Platforms without POSIX
// priority control get the plain run; the shared encode budget is still the
// primary guard against oversubscription.
func runEncodeCommand(cmd *exec.Cmd) error {
	return cmd.Run()
}
