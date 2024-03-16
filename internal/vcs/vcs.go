package vcs

import (
	"fmt"
	"runtime/debug"
	"strings"
)

func Version() string {
	var (
		time string
		revision string
		modified bool
		version string
	)

	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.time":
				time = s.Value
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					modified = true
				}
			case "-ldflags":
				if len(strings.Split(s.Value, "=")) == 2 {
					version = strings.Split(s.Value, "=")[1]
				} else {
					version = ""
				}
			}
		}
	}

	if version != "" {
		return version
	}

	if modified {
		return fmt.Sprintf("%s-%s-dirty", time, revision)
	}

	return fmt.Sprintf("%s-%s", time, revision)
}
