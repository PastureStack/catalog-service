package utils

import (
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/PastureStack/catalog-service/utils/version"
)

func VersionBetween(a, b, c string) bool {
	if a == "" && c == "" {
		return true
	} else if a == "" {
		return !VersionGreaterThan(b, c)
	} else if b == "" {
		return true
	} else if c == "" {
		return !VersionGreaterThan(a, b)
	}
	return !VersionGreaterThan(a, b) && !VersionGreaterThan(b, c)
}

func formatVersion(v, rng string) (string, string) {
	return strings.TrimPrefix(v, "v"), strings.ReplaceAll(rng, ",", " ")
}

func VersionSatisfiesRange(v, rng string) (bool, error) {
	v, rng = formatVersion(v, rng)
	sv, err := semver.StrictNewVersion(v)
	if err != nil {
		return false, err
	}

	for _, alternative := range strings.Split(rng, "||") {
		matches := true
		for _, expression := range strings.Fields(alternative) {
			operator := "="
			for _, candidate := range []string{">=", "<=", "!=", ">", "<", "=", "!"} {
				if strings.HasPrefix(expression, candidate) {
					operator = candidate
					expression = strings.TrimPrefix(expression, candidate)
					break
				}
			}
			target, parseErr := semver.StrictNewVersion(strings.TrimPrefix(expression, "v"))
			if parseErr != nil {
				return false, parseErr
			}
			condition := false
			switch operator {
			case "=":
				condition = sv.Equal(target)
			case "!", "!=":
				condition = !sv.Equal(target)
			case ">":
				condition = sv.GreaterThan(target)
			case ">=":
				condition = sv.GreaterThan(target) || sv.Equal(target)
			case "<":
				condition = sv.LessThan(target)
			case "<=":
				condition = sv.LessThan(target) || sv.Equal(target)
			}
			if !condition {
				matches = false
				break
			}
		}
		if matches {
			return true, nil
		}
	}
	return false, nil
}

func VersionGreaterThan(a, b string) bool {
	return version.GreaterThan(a, b)
}
