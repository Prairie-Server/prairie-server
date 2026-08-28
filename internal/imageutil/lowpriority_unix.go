//go:build unix

package imageutil

import (
	"os/exec"
	"syscall"
)

// encodeNiceness is the scheduling niceness applied to artwork encode
// subprocesses. Artwork is deferrable background work, so it must lose the CPU
// to playback ffmpeg (niceness 0) whenever the two overlap. 10 keeps roughly a
// tenth of the scheduler weight of a normal-priority process while still
// letting encodes run at full speed on an otherwise idle host.
const encodeNiceness = 10

// runEncodeCommand runs an artwork encode subprocess at a lowered CPU priority
// so playback ffmpeg wins contention. Priority is applied right after start;
// threads the encoder spawns afterwards inherit it.
func runEncodeCommand(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if proc := cmd.Process; proc != nil {
		// Best effort: lowering own-process priority never needs privileges,
		// but a restrictive sandbox may still refuse it.
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, proc.Pid, encodeNiceness)
	}
	return cmd.Wait()
}
