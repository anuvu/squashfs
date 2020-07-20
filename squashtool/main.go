package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/anuvu/squashfs"
	"github.com/urfave/cli/v2"
)

// defined in Makefile via '-ldflags "-X main.version="'
var version string

func printWalker(path string, info squashfs.FileInfo, err error) error {
	if err != nil {
		return err
	}
	fmt.Println(info.String())
	return nil
}

func listMain(c *cli.Context) error {
	var fname string
	if c.Args().Len() >= 1 {
		fname = c.Args().First()
	} else {
		return fmt.Errorf("Must give name of squashfs file")
	}
	path := c.Args().Get(2)
	if path == "" {
		path = "/"
	}

	s, err := squashfs.OpenSquashfs(fname)
	if err != nil {
		return fmt.Errorf("error opening squashfs: %s", err)
	}

	err = s.Walk(path, printWalker)
	if err != nil {
		return fmt.Errorf("walk %s failed: %s", path, err)
	}

	return err
}

func extractMain(c *cli.Context) error {
	var err error

	if c.Args().Len() != 2 {
		return fmt.Errorf("Expected 2 args (squashfs and out-dir), got %d", c.Args().Len())
	}
	args := c.Args().Slice()
	fname := args[0]
	outDir := args[1]
	path := c.String("path")

	s, err := squashfs.OpenSquashfs(fname)
	if err != nil {
		return fmt.Errorf("error opening squashfs: %s", err)
	}

	name2level := map[string]int{
		"quiet":   0,
		"info":    1,
		"verbose": 2,
		"debug":   3,
	}

	var level int
	var ok bool

	if level, ok = name2level[c.String("log-level")]; !ok {
		return fmt.Errorf("do not know log-level value '%s'. Needs one of: %v",
			c.String("log-level"), name2level)
	}

	logger := squashfs.PrintfLogger{Verbosity: level}

	logger.Info("Extracting squashfs file %s to %s.", fname, outDir)

	if err = os.Mkdir(outDir, squashfs.DefaultDirPerm); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}

	extractor := squashfs.Extractor{
		Path:      path,
		Dir:       outDir,
		SquashFs:  s,
		Logger:    logger,
		Owners:    c.Bool("owners"),
		Perms:     c.Bool("perms"),
		Devs:      c.Bool("devs"),
		Sockets:   c.Bool("sockets"),
		WhiteOuts: c.Bool("whiteouts"),
	}

	return extractor.Extract()
}

func versionMain(c *cli.Context) error {
	fmt.Println(version)
	return nil
}

func testMain(c *cli.Context) error {
	var fname string
	if c.Args().Len() >= 1 {
		fname = c.Args().First()
	} else {
		return fmt.Errorf("Must give name of squashfs file")
	}
	s, err := squashfs.OpenSquashfs(fname)
	if err != nil {
		return fmt.Errorf("error opening squashfs: %s", err)
	}

	var f *squashfs.File
	if f, err = squashfs.Open("README.md", &s); err != nil {
		return fmt.Errorf("error finding file: %s", err)
	}

	fmt.Printf("f is %s offset=%d\n", f.Name(), f.Pos)
	buf := make([]byte, 1024)
	rlen, err := f.Read(buf)
	if err != nil {
		fmt.Printf("got error: %s", err)
		os.Exit(1)
	}

	fmt.Printf("Read %d bytes", rlen)
	fmt.Printf("%s", string(buf))

	fmt.Println("===== top level list ====")
	if f, err = squashfs.Open("/", &s); err != nil {
		return fmt.Errorf("failed to open /")
	}
	fmt.Printf("reading %s\n", f.Name())
	names, err := f.Readdirnames(1)
	if err != nil {
		return fmt.Errorf("failed to read %s", f.Name())
	}
	fmt.Printf("has entries: %v\n", names)
	names, err = f.Readdirnames(1)
	if err != nil {
		return fmt.Errorf("error with readdirnames(1)")
	}
	fmt.Printf("has entries: %v\n", names)
	f.Close()

	if f, err = squashfs.Open("/", &s); err != nil {
		return fmt.Errorf("failed to open /")
	}
	names, err = f.Readdirnames(0)
	if err != nil {
		return fmt.Errorf("read returned %s: %v\n", err, names)
	}

	if f, err = squashfs.Open("/", &s); err != nil {
		return fmt.Errorf("failed to open /")
	}

	infos, _ := f.Readdir(0)
	for i, info := range infos {
		fmt.Printf("%d: %#v\n", i, info)
		stat := info.Sys().(syscall.Stat_t)
		fmt.Printf("uid=%d gid=%d\n", stat.Uid, stat.Gid)
	}
	fmt.Printf("mode=%s\n", infos[0].Mode())

	return nil
}

func main() {
	app := &cli.App{
		Name:    "squashtool",
		Version: version,
		Usage:   "Play around or test squash",
		Commands: []*cli.Command{
			&cli.Command{
				Name:   "version",
				Usage:  "Print version, exit",
				Action: versionMain,
			},
			&cli.Command{
				Name:   "test-main",
				Usage:  "just run the main test",
				Action: testMain,
			},
			&cli.Command{
				Name:   "list",
				Usage:  "list contents of a squashfs",
				Action: listMain,
			},
			&cli.Command{
				Name:   "extract",
				Usage:  "extract contents of a squashfs to a directory",
				Action: extractMain,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "path",
						Value: "/",
						Usage: "Start at PATH",
					},
					&cli.BoolFlag{
						Name:  "devs",
						Value: false,
						Usage: "Extract devices (mknod)",
					},
					&cli.BoolFlag{
						Name:  "sockets",
						Value: false,
						Usage: "Extract sockets (unix domain sockets)",
					},
					&cli.BoolFlag{
						Name:  "perms",
						Value: false,
						Usage: "Extract file permissions (chmod)",
					},
					&cli.BoolFlag{
						Name:  "owners",
						Value: false,
						Usage: "Extract file owners (chown)",
					},
					&cli.BoolFlag{
						Name:  "whiteouts",
						Value: false,
						Usage: "Apply whiteout files during extraction",
					},
					&cli.StringFlag{
						Name:  "log-level",
						Value: "info",
						Usage: "Change level of verbosity: quiet, info, verbose, debug",
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
