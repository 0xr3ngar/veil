package dns

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"syscall"
)

func DropPrivileges() error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil
	}

	if os.Getuid() != 0 {
		return nil
	}

	nobody, err := user.Lookup("nobody")
	if err != nil {
		return fmt.Errorf("looking up nobody user: %w", err)
	}

	uid, err := strconv.Atoi(nobody.Uid)
	if err != nil {
		return fmt.Errorf("parsing uid: %w", err)
	}
	gid, err := strconv.Atoi(nobody.Gid)
	if err != nil {
		return fmt.Errorf("parsing gid: %w", err)
	}

	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}

	log.Printf("dropped privileges to uid=%d gid=%d", uid, gid)
	return nil
}
