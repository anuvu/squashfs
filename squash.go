package squashfs

// #cgo pkg-config: libsquashfs1
// #include <string.h>
// #include <errno.h>
// #include <stdlib.h>
// #include <sqfs/compressor.h>
// #include <sqfs/dir.h>
// #include <sqfs/io.h>
// #include <sqfs/super.h>
// #include <sqfs/inode.h>
// #include <sqfs/dir_reader.h>
// #include <sqfs/id_table.h>
// #include <sqfs/data_reader.h>
import "C"

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const maxUint32 = 4294967295

// SkipDir - Just re-used from filepath
var SkipDir = filepath.SkipDir

// WalkFunc - same as filepath.WalkFunc
type WalkFunc func(string, FileInfo, error) error

// ErrNotImplemented - not implemented
var ErrNotImplemented = errors.New("not implemented")

type SquashFs struct {
	Filename   string
	file       *C.sqfs_file_t
	super      *C.sqfs_super_t
	config     *C.sqfs_compressor_config_t
	compressor *C.sqfs_compressor_t
	idTable    *C.sqfs_id_table_t
	dirReader  *C.sqfs_dir_reader_t
	dataReader *C.sqfs_data_reader_t
	root       *C.sqfs_inode_generic_t
}

func (s *SquashFs) Free() {
	// TODO: implement
}

func (s *SquashFs) Close() {
	// TODO: implement
}

func (s *SquashFs) OpenFile(name string) (*File, error) {
	return Open(name, s)
}

// Stat - os.File.Stat
func (s *SquashFs) Stat(name string) (FileInfo, error) {
	f, err := s.OpenFile(name)
	if err != nil {
		return FileInfo{}, err
	}
	return f.Stat()
}

// Lstat - os.File.Lstat
func (s *SquashFs) Lstat(name string) (FileInfo, error) {
	f, err := s.OpenFile(name)
	if err != nil {
		return FileInfo{}, err
	}
	return f.Lstat()
}

// Walk - mimics filepath.Walk
func (s *SquashFs) Walk(root string, walkFn WalkFunc) error {
	f, err := s.OpenFile(root)
	if err != nil {
		return err
	}
	info, err := f.Lstat()
	if err != nil {
		err = walkFn(root, FileInfo{Filename: root}, err)
	} else {
		err = walk(root, info, walkFn)
	}
	if err == SkipDir {
		return nil
	}
	return err
}

func walk(path string, info FileInfo, walkFn WalkFunc) error {
	if !info.IsDir() {
		return walkFn(path, info, nil)
	}
	sqfs := info.File.SquashFs

	names, err := info.File.Readdirnames(0)
	err1 := walkFn(path, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := sqfs.Lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != SkipDir {
				return err
			}
		} else {
			err = walk(filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// FileInfo - Implements a os.FileInfo interface for squash file.
type FileInfo struct {
	Filename      string
	FSize         int64
	FMode         os.FileMode
	FModTime      time.Time
	File          *File
	SymlinkTarget string
}

// Name - os.FileInfo.Name base name of the file
func (f FileInfo) Name() string {
	return path.Base(f.Filename)
}

// Size - os.FileInfo.Size length in bytes for regular files; system-dependent for others
func (f FileInfo) Size() int64 {
	return f.FSize
}

// Mode - os.FileInfo.Mode file mode bits
func (f FileInfo) Mode() os.FileMode {
	return f.FMode
}

// ModTime - os.FileInfo.ModTime modification time
func (f FileInfo) ModTime() time.Time {
	return f.FModTime
}

// IsDir - os.FileInfo.IsDir abbreviation for Mode().IsDir()
func (f FileInfo) IsDir() bool {
	return f.FMode.IsDir()
}

// Sys - os.FileInfo.Sys underlying data source (can return nil)
//       returns a syscall.Stat_t
func (f FileInfo) Sys() interface{} {
	inode := f.File.inode
	noImpl := uint64(999999)
	brokenID := C.sqfs_u32(maxUint32) // brokenID set to max value
	nlinks := C.uint(1)
	mtime := syscall.Timespec{Sec: f.ModTime().Unix()}
	inoNum := uint64(inode.base.inode_number)
	blksize := f.File.SquashFs.super.block_size
	devNo := C.uint(0)

	dataPtr := unsafe.Pointer(&inode.data)

	switch itype := inode.base._type; itype {
	case C.SQFS_INODE_BDEV:
		data := (*C.sqfs_inode_dev_t)(dataPtr)
		devNo = C.uint(data.devno)
	case C.SQFS_INODE_CDEV:
		data := (*C.sqfs_inode_dev_t)(dataPtr)
		devNo = C.uint(data.devno)
	case C.SQFS_INODE_EXT_DIR:
		data := (*C.sqfs_inode_dir_ext_t)(dataPtr)
		nlinks = data.nlink
	case C.SQFS_INODE_EXT_FILE:
		data := (*C.sqfs_inode_file_ext_t)(dataPtr)
		nlinks = data.nlink
	case C.SQFS_INODE_EXT_SLINK:
		data := (*C.sqfs_inode_slink_ext_t)(dataPtr)
		nlinks = data.nlink
	case C.SQFS_INODE_EXT_BDEV:
		data := (*C.sqfs_inode_dev_ext_t)(dataPtr)
		nlinks = data.nlink
		devNo = C.uint(data.devno)
	case C.SQFS_INODE_EXT_CDEV:
		data := (*C.sqfs_inode_dev_ext_t)(dataPtr)
		nlinks = data.nlink
		devNo = C.uint(data.devno)
	case C.SQFS_INODE_EXT_FIFO:
		data := (*C.sqfs_inode_ipc_ext_t)(dataPtr)
		nlinks = data.nlink
	case C.SQFS_INODE_EXT_SOCKET:
		data := (*C.sqfs_inode_ipc_ext_t)(dataPtr)
		nlinks = data.nlink
	}

	idtbl := f.File.SquashFs.idTable
	var uid, gid C.sqfs_u32

	if r := C.sqfs_id_table_index_to_id(idtbl, inode.base.uid_idx, &uid); r != 0 {
		uid = C.sqfs_u32(brokenID)
	}

	if r := C.sqfs_id_table_index_to_id(idtbl, inode.base.gid_idx, &gid); r != 0 {
		gid = C.sqfs_u32(brokenID)
	}

	s := syscall.Stat_t{
		Dev:     uint64(noImpl),  // ID of device containing file
		Ino:     inoNum,          // inode number
		Nlink:   uint64(nlinks),  // number of hard links
		Mode:    uint32(f.FMode), // protection
		Uid:     uint32(uid),     // user ID of owner
		Gid:     uint32(gid),     // group ID of owner
		Rdev:    uint64(devNo),   // device ID (if special file)
		Size:    f.FSize,         // total size, in bytes
		Blksize: int64(blksize),  // blocksize for file system I/O (default squash block size is 128K)
		Blocks:  f.FSize / 512,   // number of 512B blocks allocated
		Atim:    mtime,           // time of last access
		Mtim:    mtime,           // time of last modification
		Ctim:    mtime,           // time of last status change
	}

	return s
}

// String - convert to string (as in ls -l)
func (f FileInfo) String() string {
	var sizeOrMajMin string

	sys := f.Sys().(syscall.Stat_t)
	name := f.Filename
	if f.IsDir() && f.Filename != "/" {
		name += "/"
	}

	linkTarget := ""
	if f.SymlinkTarget != "" {
		linkTarget = " -> " + f.SymlinkTarget
	}

	if f.FMode&os.ModeDevice != 0 || f.FMode&os.ModeCharDevice != 0 {
		sizeOrMajMin = fmt.Sprintf("%5d, %5d", sys.Rdev/256, sys.Rdev%256)
	} else {
		sizeOrMajMin = fmt.Sprintf("%12d", f.FSize)
	}

	return fmt.Sprintf(
		"%11s %2d %4d %4d %s %s %s%s",
		f.FMode.String(), sys.Nlink, sys.Uid, sys.Gid,
		sizeOrMajMin, f.FModTime.Format("Jan  2 15:04"),
		name, linkTarget)
}

// File - a os.File for squash
type File struct {
	Filename   string
	SquashFs   *SquashFs
	Pos        int64
	size       int64
	inode      *C.sqfs_inode_generic_t
	dirReader  *C.sqfs_dir_reader_t
	dataReader *C.sqfs_data_reader_t
}

// Open - os.File.Open
func Open(name string, squash *SquashFs) (*File, error) {
	var inode *C.sqfs_inode_generic_t
	if name != "/" && strings.HasSuffix(name, "/") {
		name = name[:len(name)-1]
	}
	f := File{Filename: name, SquashFs: squash, Pos: 0, size: -1}
	if r := C.sqfs_dir_reader_find_by_path(squash.dirReader, squash.root, C.CString(name), &inode); r != 0 {
		return &f, os.ErrNotExist
	}
	f.inode = inode
	return &f, nil
}

// Close - os.File.Close
func (f *File) Close() error {
	if f.dirReader != nil {
		C.free(unsafe.Pointer(f.dirReader))
	}
	return ErrNotImplemented
}

// Fd - os.File.Fd
func (f *File) Fd() uintptr {
	// TODO: implement
	return 1
}

// Name - os.File.Name
func (f *File) Name() string {
	return f.Filename
}

// Read - os.File.Read
func (f *File) Read(b []byte) (int, error) {
	if f.dataReader == nil {
		f.dataReader = f.SquashFs.dataReader.Copy()
		if f.dataReader == nil {
			return 0, fmt.Errorf("failed to create a datareader for %s", f.Filename)
		}
	}
	if f.Pos == f.Size() {
		return 0, io.EOF
	}
	rlen := C.sqfs_data_reader_read(f.SquashFs.dataReader,
		f.inode, C.ulong(f.Pos), unsafe.Pointer(&b[0]), C.uint(len(b)))
	if rlen < 0 {
		return int(rlen), fmt.Errorf("Error reading from %s. got error code %d", f.Filename, rlen)
	}

	f.Pos += int64(rlen)

	return int(rlen), nil
}

// Size - mostly just a convienence, used by Seek.
func (f *File) Size() int64 {
	if f.size == -1 {
		if fi, err := getFileInfo(f); err != nil {
			f.size = 0
		} else {
			f.size = fi.Size()
		}
	}
	return f.size
}

// Name - return the name entry in a sqfs_dir_entry_t struct.
func (ent *C.sqfs_dir_entry_t) Name() string {
	// sqfs_dir_entry_t is at
	// https://github.com/AgentD/squashfs-tools-ng/blob/master/include/sqfs/dir.h#L74
	// but cgo can't get to ent.name - compiler gives:
	//    undefined (type *_Ctype_struct_sqfs_dir_entry_t has no field or method name
	// so instead get GoBytes of the size of the struct (which does *not* include name)
	// and add ent.size + 1 (size is off-by-one per doc) and then return the string.
	structSize := C.int(C.sizeof_sqfs_dir_entry_t)
	b := C.GoBytes(unsafe.Pointer(ent), structSize+C.int(ent.size)+1)
	return string(b[structSize:])
}

// symlinkTarget - return the target of this inode. empty string if not a link.
func (inode *C.sqfs_inode_generic_t) symlinkTarget() string {
	var dataPtr = unsafe.Pointer(&inode.data)
	var targetSize C.int
	structSize := C.int(C.sizeof_sqfs_inode_generic_t)

	switch itype := inode.base._type; itype {
	case C.SQFS_INODE_SLINK:
		targetSize = C.int((*C.sqfs_inode_slink_t)(dataPtr).target_size)
	case C.SQFS_INODE_EXT_SLINK:
		targetSize = C.int((*C.sqfs_inode_slink_ext_t)(dataPtr).target_size)
	default:
		return ""
	}

	b := C.GoBytes(unsafe.Pointer(inode), structSize+targetSize)
	return string(b[structSize:])
}

func getFileInfo(f *File) (FileInfo, error) {
	inode := f.inode

	var myMode os.FileMode
	var size int64

	dataPtr := unsafe.Pointer(&(inode.data))

	switch itype := inode.base._type; itype {
	case C.SQFS_INODE_DIR:
		myMode |= os.ModeDir
		data := (*C.sqfs_inode_dir_t)(dataPtr)
		size = int64(data.size)
	case C.SQFS_INODE_FILE:
		data := (*C.sqfs_inode_file_t)(dataPtr)
		size = int64(data.file_size)
	case C.SQFS_INODE_SLINK:
		myMode |= os.ModeSymlink
		data := (*C.sqfs_inode_slink_t)(dataPtr)
		size = int64(data.target_size)
	case C.SQFS_INODE_BDEV:
		myMode |= os.ModeDevice
		// data := (*C.sqfs_inode_dev_t)(dataPtr)
		size = 0
	case C.SQFS_INODE_CDEV:
		myMode |= os.ModeCharDevice
		// data := (*C.sqfs_inode_dev_t)(dataPtr)
		size = 0
	case C.SQFS_INODE_FIFO:
		myMode |= os.ModeNamedPipe
		// data := (*C.sqfs_inode_ipc_t)(dataPtr)
	case C.SQFS_INODE_SOCKET:
		myMode |= os.ModeSocket
		// data := (*C.sqfs_inode_ipc_t)(dataPtr)
	case C.SQFS_INODE_EXT_DIR:
		myMode |= os.ModeDir
		data := (*C.sqfs_inode_dir_ext_t)(dataPtr)
		size = int64(data.size)
	case C.SQFS_INODE_EXT_FILE:
		data := (*C.sqfs_inode_file_ext_t)(dataPtr)
		size = int64(data.file_size)
	case C.SQFS_INODE_EXT_SLINK:
		data := (*C.sqfs_inode_slink_ext_t)(dataPtr)
		size = int64(data.target_size)
	case C.SQFS_INODE_EXT_BDEV:
		// data := (*C.sqfs_inode_dev_ext_t)(dataPtr)
		myMode |= os.ModeDevice
		size = 0
	case C.SQFS_INODE_EXT_CDEV:
		// data := (*C.sqfs_inode_dev_ext_t)(dataPtr)
		myMode |= os.ModeCharDevice
		size = 0
	case C.SQFS_INODE_EXT_FIFO:
		// data := (*C.sqfs_inode_ipc_ext_t)(dataPtr)
		myMode |= os.ModeNamedPipe
		size = 0
	case C.SQFS_INODE_EXT_SOCKET:
		// data := (*C.sqfs_inode_ipc_ext_t)(dataPtr)
		myMode |= os.ModeSocket
		size = 0
	}

	myMode |= os.FileMode(inode.base.mode)

	fInfo := FileInfo{
		Filename:      f.Filename,
		FModTime:      time.Unix(int64(inode.base.mod_time), 0),
		FSize:         size,
		FMode:         myMode,
		File:          f,
		SymlinkTarget: inode.symlinkTarget(),
	}
	return fInfo, nil
}

// Readdir - os.File.Readdir
func (f *File) Readdir(n int) ([]os.FileInfo, error) {
	infos := []os.FileInfo{}
	names, rdErr := f.Readdirnames(n)
	if rdErr != nil && rdErr != io.EOF {
		return infos, rdErr
	}
	var curFile *File
	var curInfo FileInfo
	var err error
	for _, name := range names {
		if curFile, err = Open(f.Filename+name, f.SquashFs); err != nil {
			return infos, err
		}
		defer curFile.Close()

		if curInfo, err = curFile.Stat(); err != nil {
			return infos, err
		}
		infos = append(infos, curInfo)
	}
	return infos, rdErr
}

func (rd *C.sqfs_data_reader_t) Copy() *C.sqfs_data_reader_t {
	return (*C.sqfs_data_reader_t)(unsafe.Pointer(C.sqfs_copy(unsafe.Pointer(rd))))
}

func (rd *C.sqfs_dir_reader_t) Copy() *C.sqfs_dir_reader_t {
	return (*C.sqfs_dir_reader_t)(unsafe.Pointer(C.sqfs_copy(unsafe.Pointer(rd))))
}

// Readdirnames - os.File.Readdirnames
func (f *File) Readdirnames(n int) ([]string, error) {
	var ent *C.sqfs_dir_entry_t
	names := []string{}
	if f.dirReader == nil {
		f.dirReader = f.SquashFs.dirReader.Copy()
		if r := C.sqfs_dir_reader_open_dir(f.dirReader, f.inode, 0); r != 0 {
			return names, fmt.Errorf("unexpected error %d. %s not a dir?", r, f.Name())
		}
	}
	for i := 0; n <= 0 || i < n; i++ {
		r := C.sqfs_dir_reader_read(f.dirReader, &ent)
		if r > 0 {
			if n > 0 {
				return names, io.EOF
			}
			return names, nil
		} else if r < 0 {
			return names, fmt.Errorf("Error while reading %s (%d)", f.Filename, r)
		}

		names = append(names, ent.Name())
	}
	return names, nil
}

// Seek - os.File.Seek
func (f *File) Seek(offset int64, whence int) (ret int64, err error) {
	var ref = int64(0)

	switch whence {
	case io.SeekStart:
		ref = 0
	case io.SeekCurrent:
		ref = f.Pos
	case io.SeekEnd:
		ref = f.Size()
	}

	newPos := ref + offset
	if newPos < 0 {
		return f.Pos, fmt.Errorf("Cannot seek to < 0 in %s (pos=%d ref=%d offset=%d size=%d)",
			f.Filename, f.Pos, ref, offset, f.Size())
	} else if newPos > f.Size() {
		return f.Pos,
			fmt.Errorf("Cannot seek past end of file in %s (pos=%d ref=%d offset=%d size=%d)",
				f.Filename, f.Pos, ref, offset, f.Size())
	}
	f.Pos = newPos
	return f.Pos, nil
}

// Stat - os.File.Stat - If file is a symlink, tell about the target.
func (f *File) Stat() (FileInfo, error) {
	info, err := getFileInfo(f)
	if err != nil || info.FMode&os.ModeSymlink != 0 {
		return info, err
	}

	tfile, tErr := f.SquashFs.OpenFile(info.SymlinkTarget)
	if tErr != nil {
		return FileInfo{}, err
	}

	return getFileInfo(tfile)
}

// Lstat - os.File.Lstat - if file is a symlink info is about the link not the target.
func (f *File) Lstat() (FileInfo, error) {
	return getFileInfo(f)
}

// Sync - os.File.Sync
func (f *File) Sync() error {
	// read only, sync is noop
	return nil
}

// OpenSquashfs - return a SquashFs struct for fname.
func OpenSquashfs(fname string) (SquashFs, error) {
	var err error
	sqfs := SquashFs{Filename: fname}
	sqfs.super = (*C.sqfs_super_t)(C.malloc(C.sizeof_sqfs_super_t))
	sqfs.config = (*C.sqfs_compressor_config_t)(C.malloc(C.sizeof_sqfs_compressor_config_t))

	if sqfs.file, err = C.sqfs_open_file(C.CString(fname), C.SQFS_FILE_OPEN_READ_ONLY); err != nil {
		sqfs.Free()
		return SquashFs{}, fmt.Errorf("failed to open %s: %s", fname, err)
	}

	if _, err = C.sqfs_super_read(sqfs.super, sqfs.file); err != nil {
		sqfs.Free()
		return SquashFs{}, fmt.Errorf("failed to open %s: %s", fname, err)
	}

	C.sqfs_compressor_config_init(sqfs.config, C.SQFS_COMPRESSOR(sqfs.super.compression_id),
		C.ulong(sqfs.super.block_size), C.SQFS_COMP_FLAG_UNCOMPRESS)

	if r := C.sqfs_compressor_create(sqfs.config, &sqfs.compressor); r != 0 {
		sqfs.Free()
		return sqfs, fmt.Errorf("error creating compressor: %d", r)
	}

	if sqfs.idTable, err = C.sqfs_id_table_create(0); err != nil {
		sqfs.Free()
		return sqfs, fmt.Errorf("error creating id table: %d", err)
	}

	if r := C.sqfs_id_table_read(sqfs.idTable, sqfs.file, sqfs.super, sqfs.compressor); r != 0 {
		sqfs.Free()
		return sqfs, fmt.Errorf("error loading ID table")
	}

	/* create a directory reader and get the root inode */

	sqfs.dirReader = C.sqfs_dir_reader_create(sqfs.super, sqfs.compressor, sqfs.file, 0)
	if sqfs.dirReader == nil {
		sqfs.Free()
		return sqfs, fmt.Errorf("error creating directory reader")
	}

	var wd *C.sqfs_inode_generic_t
	if r := C.sqfs_dir_reader_get_root_inode(sqfs.dirReader, &wd); r != 0 {
		sqfs.Free()
		return sqfs, fmt.Errorf("error getting root inode")
	}

	/* create a data reader */
	sqfs.dataReader = C.sqfs_data_reader_create(sqfs.file, C.ulong(sqfs.super.block_size), sqfs.compressor, 0)
	if sqfs.dataReader == nil {
		sqfs.Free()
		return sqfs, fmt.Errorf("error creating data reader")
	}

	if r := C.sqfs_data_reader_load_fragment_table(sqfs.dataReader, sqfs.super); r != 0 {
		sqfs.Free()
		return sqfs, fmt.Errorf("error loading fragment table")
	}

	if r := C.sqfs_dir_reader_get_root_inode(sqfs.dirReader, &sqfs.root); r != 0 {
		sqfs.Free()
		return sqfs, fmt.Errorf("error finding root node")
	}

	return sqfs, nil
}
