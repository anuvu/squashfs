package squashfs

// #cgo LDFLAGS: -ldl
// #include <stdlib.h>
// #include <dlfcn.h>
// #include <sys/types.h>
// #include <sys/stat.h>
// #include <fcntl.h>
// #include <unistd.h>
// #include <errno.h>
// #include <stdio.h>
//
// int
// my_mknod(void *f, const char *s, mode_t mode, dev_t dev)
// {
//   int (*xmknod)(int, const char *, mode_t mode, dev_t dev);
//   printf("trying %s %d %d\n", s, (int)mode, (int)dev);
//
//   xmknod = (int (*)(int, const char *, mode_t mode, dev_t dev))f;
//   return xmknod(_MKNOD_VER_LINUX, s, mode, dev);
// }
import "C"

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func GetLibcFsOps() (FsOps, error) {
	x := LibcFsOps{}
	err := x.init()
	return &x, err
}

type LibcFsOps struct {
	chmod  unsafe.Pointer
	chown  unsafe.Pointer
	xmknod unsafe.Pointer
}

func (l *LibcFsOps) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (l *LibcFsOps) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

func (l *LibcFsOps) Mknod(path string, info FileInfo) error {
	stat := info.Sys().(syscall.Stat_t)
	d := C.dev_t(stat.Rdev)
	m := C.mode_t(DefaultFilePerm)
	p := C.CString(path)

	fmt.Printf("path=%v m=%v d=%v\n", p, m, d)
	eno := C.my_mknod(l.xmknod, p, m, d)
	if eno != 0 {
		return fmt.Errorf("Got error for %s: %d", path, eno)
	}
	return nil
}

func (l *LibcFsOps) init() error {
	handle, err := GetHandle([]string{"libfakeroot-sysv.so"})
	if err != nil {
		return err
	}

	// libc 'mknod' is a macro , the function call is actually __xmknod
	l.xmknod, err = handle.GetSymbolPointer("__xmknod")
	if err != nil {
		return err
	}

	l.chmod, err = handle.GetSymbolPointer("chmod")
	if err != nil {
		return err
	}

	l.chown, err = handle.GetSymbolPointer("chown")
	if err != nil {
		return err
	}

	return nil
}

// copied from https://github.com/coreos/pkg/dlopen

var ErrSoNotFound = errors.New("unable to open a handle to the library")

// LibHandle represents an open handle to a library (.so)
type LibHandle struct {
	Handle  unsafe.Pointer
	Libname string
}

// GetHandle tries to get a handle to a library (.so), attempting to access it
// by the names specified in libs and returning the first that is successfully
// opened. Callers are responsible for closing the handler. If no library can
// be successfully opened, an error is returned.
func GetHandle(libs []string) (*LibHandle, error) {
	for _, name := range libs {
		libname := C.CString(name)
		defer C.free(unsafe.Pointer(libname))
		handle := C.dlopen(libname, C.RTLD_LAZY)
		if handle != nil {
			h := &LibHandle{
				Handle:  handle,
				Libname: name,
			}
			return h, nil
		}
	}
	return nil, ErrSoNotFound
}

// GetSymbolPointer takes a symbol name and returns a pointer to the symbol.
func (l *LibHandle) GetSymbolPointer(symbol string) (unsafe.Pointer, error) {
	sym := C.CString(symbol)
	defer C.free(unsafe.Pointer(sym))

	C.dlerror()
	p := C.dlsym(l.Handle, sym)
	e := C.dlerror()
	if e != nil {
		return nil, fmt.Errorf("error resolving symbol %q: %v", symbol, errors.New(C.GoString(e)))
	}

	return p, nil
}

// Close closes a LibHandle.
func (l *LibHandle) Close() error {
	C.dlerror()
	C.dlclose(l.Handle)
	e := C.dlerror()
	if e != nil {
		return fmt.Errorf("error closing %v: %v", l.Libname, errors.New(C.GoString(e)))
	}

	return nil
}
