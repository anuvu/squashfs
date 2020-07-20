package squashfs

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// DefaultFilePerm - files will be created with this perm by default.
const DefaultFilePerm = 0644

// DefaultDirPerm - Directories created with this perm
const DefaultDirPerm = 0755

// OpenDirPerm - Perm that current uid can write to
const OpenDirPerm = 0777

type Extractor struct {
	Dir       string
	SquashFs  SquashFs
	Path      string
	WhiteOuts bool
	Owners    bool
	Perms     bool
	Devs      bool
	Sockets   bool
	Logger    Logger
}

// Extract - extract the
func (e *Extractor) Extract() error {
	e.Logger.Debug("extractor: %#v", e)
	return e.SquashFs.Walk(e.Path, e.extract)
}

func (e *Extractor) extract(path string, info FileInfo, perr error) error {
	if perr != nil {
		e.Logger.Info("extract called with %s info=%s and %s", path, info.Filename, perr)
		return perr
	}

	if whiteOut := getWhiteOut(info); whiteOut != "" {
		if !e.WhiteOuts {
			e.Logger.Debug("skipping whiteout %s", path)
			return nil
		}
		return e.applyWhiteOut(path, whiteOut)
	}

	mode := info.FMode

	var err error
	if mode&os.ModeDir != 0 {
		err = e.extractDir(path, info)
	} else if mode&os.ModeSymlink != 0 {
		err = e.extractSymlink(path, info)
	} else if mode&os.ModeSocket != 0 {
		if !e.Sockets {
			e.Logger.Debug("skipping socket %s", path)
			return nil
		}
		err = e.extractSocket(path, info)
	} else if mode&os.ModeNamedPipe != 0 {
		err = e.extractNamedPipe(path, info)
	} else if mode&os.ModeDevice != 0 {
		if !e.Devs {
			e.Logger.Debug("skipping block device node %s", path)
			return nil
		}
		err = e.extractBlockDevice(path, info)
	} else if mode&os.ModeCharDevice != 0 {
		if !e.Devs {
			e.Logger.Debug("skipping char device node %s", path)
			return nil
		}
		err = e.extractCharDevice(path, info)
	} else if mode&os.ModeIrregular != 0 {
		err = e.extractIrregular(path, info)
	} else if mode.IsRegular() {
		err = e.extractRegular(path, info)
	} else {
		return fmt.Errorf("could not determine file type of '%s'", path)
	}

	if err != nil {
		return err
	}

	fpath := filepath.Join(e.Dir, path)
	if e.Owners {
		stat := info.Sys().(syscall.Stat_t)
		e.Logger.Debug("chown(%s, %d, %d)", path, stat.Uid, stat.Gid)
		if err := chown(fpath, int(stat.Uid), int(stat.Gid)); err != nil {
			e.Logger.Info("chown(%s, %d, %d) failed: %s", path, stat.Uid, stat.Gid, err)
			return err
		}
	}

	if e.Perms {
		e.Logger.Debug("chmod(%s, %#o)", path, info.FMode.Perm())
		if err := chmod(fpath, info.FMode); err != nil {
			e.Logger.Info("chmod(%s, %#o) failed: %s", path, info.FMode.Perm(), err)
			return err
		}
	}

	return nil
}

func (e *Extractor) extractSymlink(path string, info FileInfo) error {
	e.Logger.Debug("symlink: %s", path)
	targetPath := filepath.Join(e.Dir, path)
	return doCreate(targetPath, info,
		func() error {
			return os.Symlink(info.SymlinkTarget, targetPath)
		})
}

func (e *Extractor) extractNamedPipe(path string, info FileInfo) error {
	e.Logger.Debug("mkfifo: %s", path)
	targetPath := filepath.Join(e.Dir, path)
	return doCreate(targetPath, info,
		func() error {
			return syscall.Mkfifo(targetPath, DefaultFilePerm)
		},
	)
}

func (e *Extractor) extractSocket(path string, info FileInfo) error {
	e.Logger.Debug("socket: %s", path)
	targetPath := filepath.Join(e.Dir, path)
	return doCreate(targetPath, info,
		func() error {
			_, err := net.Listen("unix", targetPath)
			return err
		})
}

func (e *Extractor) extractBlockDevice(path string, info FileInfo) error {
	e.Logger.Debug("bdev: %s", path)
	targetPath := filepath.Join(e.Dir, path)
	return doCreate(targetPath, info,
		func() error { return mknod(targetPath, info) })
}

func (e *Extractor) extractCharDevice(path string, info FileInfo) error {
	e.Logger.Debug("cdev: %s", path)
	targetPath := filepath.Join(e.Dir, path)
	return doCreate(targetPath, info,
		func() error { return mknod(targetPath, info) })
}

func (e *Extractor) extractRegular(path string, info FileInfo) error {
	var finalError, cleanupError error
	targetPath := filepath.Join(e.Dir, path)
	cleanup, err := prepWrite(targetPath, info)

	if err == nil {
		if writeFp, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, DefaultFilePerm); err == nil {
			defer writeFp.Close()
			if written, err := io.Copy(writeFp, info.File); err == nil {
				if written != info.FSize {
					finalError = fmt.Errorf("wrote %d bytes to %s. expected %d from %s",
						written, targetPath, info.FSize, path)
				}
			}
		} else {
			finalError = err
		}
	} else {
		finalError = err
	}

	cleanupError = cleanup()
	if finalError != nil {
		// there was an error before cleanup, so log the cleanup error, return the real error.
		e.Logger.Info("prepWrite cleanup for %s failed: %s", targetPath, cleanupError)
	} else {
		finalError = cleanupError
	}
	return finalError
}

func (e *Extractor) extractIrregular(path string, info FileInfo) error {
	return fmt.Errorf("cannot extract Irregular file %s", path)
}

func (e *Extractor) extractDir(path string, info FileInfo) error {
	e.Logger.Debug("mkdir %s", path)

	// we do not use doCreate here because we do not want to remove if exist.
	fpath := filepath.Join(e.Dir, path)
	if err := os.Mkdir(fpath, DefaultDirPerm); os.IsExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (e *Extractor) applyWhiteOut(path string, whiteOut string) error {
	fp := filepath.Join(e.Dir, path)
	if PathExists(filepath.Join(e.Dir, path)) {
		return os.RemoveAll(fp)
	}
	return nil
}

// doCreate - prep writing of fInfo to path, and then call creator.
func doCreate(path string, fInfo FileInfo, creator func() error) error {
	var createError, cleanupError error
	cleanup, err := prepWrite(path, fInfo)
	if err != nil {
		createError = creator()
	}
	cleanupError = cleanup()

	if cleanupError != nil {
		if createError == nil {
			return cleanupError
		}
	}

	return createError
}

// prepWrite - prepare to write to path
//   if dirname(path) is not a directory - return error
func prepWrite(path string, finfo FileInfo) (func() error, error) {
	cleanup := func() error { return nil }
	dir := filepath.Dir(path)
	dirFinfo, err := os.Stat(dir)
	if err != nil {
		// could not stat parent directory.
		return cleanup, err
	}

	if !dirFinfo.IsDir() {
		return cleanup, fmt.Errorf("dirname(%s) = %s : not a directory", path, dir)
	}

	if unix.Access(dir, unix.W_OK) != nil {
		oldPerms := dirFinfo.Mode()
		setBack := func() error {
			return chmod(dir, oldPerms)
		}
		if err := chmod(dir, OpenDirPerm); err != nil {
			return cleanup, err
		}
		if unix.Access(dir, unix.W_OK) != nil {
			if err := setBack(); err != nil {
				return cleanup, fmt.Errorf("cannot make %s writable, failed setting back", dir)
			}
			return cleanup, fmt.Errorf("cannot make %s writable", dir)
		}
		cleanup = setBack
	}

	pathFinfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		// nothing to do
		return cleanup, nil
	} else if err != nil {
		return cleanup, err
	}

	// path exists, so get rid of it.
	if pathFinfo.IsDir() {
		if finfo.IsDir() {
			// path is already a dir, leave it.
			// caller has to deal with mkdir failing with os.IsExist()
			return cleanup, nil
		}
		// path is a dir, but we want something else there so purge.
		if err := os.RemoveAll(path); err != nil {
			return cleanup, err
		}
		return cleanup, nil
	}

	// path exists, but is not a dir.
	return cleanup, os.Remove(path)
}

func isFakeroot() bool {
	return os.Getenv("FAKEROOTKEY") != ""
}

func chown(path string, uid int, gid int) error {
	// something like this will fail under fakeroot.  I think it is
	// because golang is making the syscall "directly" rather than through
	// the library that was LD_PRELOAD.
	// file := "/tmp/my.txt"
	// owner := 0
	// if err := os.Chown(file, owner, owner); err != nil {
	//	fmt.Printf("Failed chown %s %d: %s\n", file, owner, err)
	//	os.Exit(1)
	// }
	if !isFakeroot() {
		return os.Lchown(path, uid, gid)
	}
	cmd := []string{"chown", "--no-dereference", fmt.Sprintf("%d:%d", uid, gid), path}
	return exec.Command(cmd[0], cmd[1:]...).Run()
}

func chmod(path string, mode os.FileMode) error {
	if mode&os.ModeSymlink != 0 {
		// you can't chmod a symlink.
		return nil
	}
	return os.Chmod(path, mode.Perm())
}

func mknod(path string, info FileInfo) error {
	stat := info.Sys().(syscall.Stat_t)
	if !isFakeroot() {
		return syscall.Mknod(path, DefaultFilePerm, int(stat.Rdev))
	}

	// We execute `mknod` for same fakeroot reason as chown.
	// dtype is b (block), c (char), p (fifo)
	var dtype string
	majMin := []string{fmt.Sprintf("%d", stat.Rdev/256), fmt.Sprintf("%d", stat.Rdev%256)}

	if info.FMode&os.ModeCharDevice != 0 {
		dtype = "c"
	} else if info.FMode&os.ModeDevice != 0 {
		dtype = "b"
	} else if info.FMode&os.ModeNamedPipe != 0 {
		dtype = "p"
		majMin = []string{}
	} else {
		return fmt.Errorf("%s is not a char, block or fifo", path)
	}

	cmd := append(
		[]string{"mknod", fmt.Sprintf("--mode=%o", info.FMode.Perm()),
			path, dtype}, majMin...)
	if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
		if PathExists(path) {
			return os.ErrExist
		}
		return err
	}
	return nil
}

func getWhiteOut(info FileInfo) string {
	// squashfs / overlayfs is a character device major/minor 0/0 with same name.
	if info.FMode&os.ModeCharDevice != 0 {
		stat := info.Sys().(syscall.Stat_t)
		if stat.Rdev == 0 {
			return info.Filename
		}
	}
	return ""
}

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
