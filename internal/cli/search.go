package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/luanti-org/luma/internal/update"
)

// searchTypes maps the accepted -t values to ContentDB package types.
var searchTypes = map[string]string{
	"mod": "mod", "m": "mod",
	"game": "game", "g": "game",
	"txp": "txp", "texturepack": "txp", "t": "txp",
}

func search(c *ctx, args []string) int {
	fs := c.newFlagSet()
	typeName := fs.StringP("type", "t", "", "only packages of this type: mod, game or txp (or m, g, t, texturepack)")
	namesOnly := fs.Bool("names", false, "print only author/name, one per line")
	refresh := fs.BoolP("refresh", "r", false, "fetch the package list again instead of using the cache")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}

	pkgType := ""
	if *typeName != "" {
		var ok bool
		if pkgType, ok = searchTypes[strings.ToLower(*typeName)]; !ok {
			return c.usageErr("%s: unknown type %q, expected mod, game or txp", fs.Name(), *typeName)
		}
	}

	maxAge := update.PackageListMaxAge
	if *refresh {
		maxAge = 0
	}
	packages, err := update.FetchPackages(c.cdb, c.eng, maxAge)
	if err != nil {
		return c.err("fetching package list: %v", err)
	}

	installed := update.ScanInstalledReleases(c.eng)
	matches := installed.InstalledFirst(update.MatchPackages(packages, strings.Join(fs.Args(), " "), pkgType))

	if *namesOnly {
		for _, p := range matches {
			fmt.Fprintln(c.stdout, p.Author+"/"+p.Name)
		}
		return exitOK
	}

	if len(matches) == 0 {
		fmt.Fprintln(c.stdout, "No packages found.")
		return exitOK
	}

	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	header := "\tTITLE\tPACKAGE"
	if pkgType == "" {
		header += "\tTYPE"
	}
	fmt.Fprintln(tw, header)

	for _, p := range matches {
		line := installed.Tag(p) + "\t" + p.Title + "\t" + p.Author + "/" + p.Name
		if pkgType == "" {
			line += "\t" + p.Type
		}
		fmt.Fprintln(tw, line)
	}

	tw.Flush()

	return exitOK
}
