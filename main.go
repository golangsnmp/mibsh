package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/golangsnmp/gomib"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/mibsh/internal/profile"
)

type pathList []string

func (p *pathList) String() string { return strings.Join(*p, ", ") }
func (p *pathList) Set(v string) error {
	*p = append(*p, v)
	return nil
}

func main() {
	var paths pathList
	var permissive bool
	var target string
	var community string
	var version string

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `mibsh - interactive SNMP MIB browser and query tool

Usage:
  mibsh [options] [MODULE ...]

Options:
  -p PATH             MIB search path (repeatable, recursive)
  -permissive         use permissive strictness when loading
  -target HOST[:PORT] SNMP target for queries
  -community STRING   SNMP community string (default "public")
  -version VERSION    SNMP version: 1, 2c, 3 (default "2c")

If no -p paths are given, mibsh searches standard system locations:
  - net-snmp: /usr/share/snmp/mibs, ~/.snmp/mibs, $MIBDIRS
  - libsmi:   /usr/share/mibs/*, $SMIPATH

On Debian/Ubuntu, install MIBs with:
  apt install snmp-mibs-downloader

Examples:
  mibsh                           Browse all system MIBs
  mibsh IF-MIB SNMPv2-MIB        Load specific modules only
  mibsh -p /path/to/mibs         Use a custom MIB directory
  mibsh -target 192.168.1.1      Browse MIBs and query a device

Press ? inside mibsh for key bindings.
`)
	}

	flag.Var(&paths, "p", "MIB search path (repeatable)")
	flag.BoolVar(&permissive, "permissive", false, "use permissive strictness")
	flag.StringVar(&target, "target", "", "SNMP target host[:port]")
	flag.StringVar(&community, "community", "public", "SNMP community string")
	flag.StringVar(&version, "version", "2c", "SNMP version (1, 2c, 3)")
	flag.Parse()
	modules := flag.Args()

	fmt.Fprintf(os.Stderr, "Loading MIBs...")
	m, err := loadMib(paths, modules, permissive)
	fmt.Fprintf(os.Stderr, " done.\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		if errors.Is(err, gomib.ErrNoSources) {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "No MIB files found. Either:")
			fmt.Fprintln(os.Stderr, "  - Install MIBs (e.g. apt install snmp-mibs-downloader)")
			fmt.Fprintln(os.Stderr, "  - Use -p to specify a MIB directory")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Run mibsh -h for more information.")
		}
		os.Exit(1)
	}

	cfg := appConfig{
		target:    target,
		community: community,
		version:   profile.NormalizeVersion(version),
	}

	profiles := profile.NewStore()
	var profileErr error
	if err := profiles.Load(); err != nil {
		profileErr = err
	}

	app := newApp(m, cfg, profiles)
	if profileErr != nil {
		app.initWarning = "Could not load profiles: " + profileErr.Error()
	}

	p := tea.NewProgram(app)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if mdl, ok := finalModel.(model); ok && mdl.snmp != nil {
		mdl.snmp.Close()
	}
}

func loadMib(paths []string, modules []string, permissive bool) (*mib.Mib, error) {
	var opts []gomib.LoadOption

	if len(paths) > 0 {
		var sources []gomib.Source
		for _, p := range paths {
			src, err := gomib.DirTree(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot access path %s: %v\n", p, err)
				continue
			}
			sources = append(sources, src)
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("no valid MIB sources from provided paths")
		}
		if len(sources) == 1 {
			opts = append(opts, gomib.WithSource(sources[0]))
		} else {
			opts = append(opts, gomib.WithSource(gomib.Multi(sources...)))
		}
	} else {
		opts = append(opts, gomib.WithSystemPaths())
	}

	if permissive {
		opts = append(opts, gomib.WithStrictness(mib.StrictnessPermissive))
	}

	if len(modules) > 0 {
		opts = append(opts, gomib.WithModules(modules...))
	}

	return gomib.Load(context.Background(), opts...)
}
