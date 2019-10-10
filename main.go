package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/reexec"
)

func init() {
	reexec.Register("nsInitialisation", nsInitialisation)
	if reexec.Init() {
		os.Exit(0)
	}
}

func nsInitialisation() {
	fmt.Printf("\n>> namespace setup code goes here <<\n\n")
	newRootPath := os.Args[1]

	if err := mountProc(newRootPath); err != nil {
		fmt.Printf("error mounting the proc file.. - %s\n", err)
		os.Exit(1)
	}

	if err := pivotRoot(newRootPath); err != nil {
		fmt.Printf("error pivoting to new root fs.. - %s\n", err)
		os.Exit(1)
	}

	if err := syscall.Sethostname([]byte("rushi_cont")); err != nil {
		fmt.Printf("Error setting hostname - %s\n", err)
		os.Exit(1)
	}

	if err := waitForNetwork(); err != nil {
		fmt.Printf("error waiting for network - %s\n", err)
		os.Exit(1)
	}
	nsRun()
}

func nsRun() {
	cmd := exec.Command("/bin/sh")

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = []string{"PS1=-[custom_cont]- # "}

	if err := cmd.Run(); err != nil {
		fmt.Printf("error running the /bin/sh command - %s\n", err)
		os.Exit(1)

	}
}

func pivotRoot(newroot string) error {
	putold := filepath.Join(newroot, "/.pivot_root")

	// bind mount newroot to itself - this is a slight hack
	// needed to work around a pivot_root requirement
	if err := syscall.Mount(
		newroot,
		newroot,
		"",
		syscall.MS_BIND|syscall.MS_REC,
		"",
	); err != nil {
		return err
	}

	// make directory for put old
	if err := os.MkdirAll(putold, 0700); err != nil {
		return err
	}

	// perform pivot root action
	if err := syscall.PivotRoot(newroot, putold); err != nil {
		return err
	}

	// change the current working directory to the pivoted dir
	if err := os.Chdir("/"); err != nil {
		return err
	}

	// unmount the old root file system
	putold = "/.pivot_root"
	if err := syscall.Unmount(putold, syscall.MNT_DETACH); err != nil {
		return err
	}

	// remove the old root file system......
	if err := os.RemoveAll(putold); err != nil {
		return err
	}

	return nil
}

func mountProc(newroot string) error {
	source := "proc"
	target := filepath.Join(newroot, "/proc")
	fstype := "proc"
	flags := 0
	data := ""

	os.MkdirAll(target, 0755)
	if err := syscall.Mount(
		source,
		target,
		fstype,
		uintptr(flags),
		data,
	); err != nil {
		return err
	}

	return nil
}

func waitForNetwork() error {
	maxWait := time.Second * 3
	checkInterval := time.Second
	timeStarted := time.Now()

	for {
		interfaces, err := net.Interfaces()
		if err != nil {
			return err
		}

		if len(interfaces) > 1 {
			return nil
		}

		if time.Since(timeStarted) > maxWait {
			return fmt.Errorf("time out")
		}
		time.Sleep(checkInterval)
	}
}

func main() {
	var rootfspath string = "rootfs"
	var netsetgoPath string = "/usr/local/bin/netsetgo"
	cmd := reexec.Command("nsInitialisation", rootfspath)
	println("running bash bruh.")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = []string{"PS1=-[ns-process]- # "}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER,

		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("error running /bin/sh command - %s\n", err)
		os.Exit(1)
	}

	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	netsetgoCmd := exec.Command(netsetgoPath, "-pid", pid)
	if err := netsetgoCmd.Run(); err != nil {
		fmt.Printf("Error running the netsetgo func")
		os.Exit(1)
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("Error waiting for reexec command - %s\n", err)
		os.Exit(1)
	}
}
