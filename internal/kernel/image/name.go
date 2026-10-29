// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image

import "regexp"

// The distribution spec's grammars for repository names and tags.
var (
	// repositoryPattern is the distribution spec's repository name grammar.
	repositoryPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:\.|_|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:\.|_|__|-+)[a-z0-9]+)*)*$`)
	// tagPattern is the distribution spec's tag grammar.
	tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
)

// maxRepository is the longest repository name the spec allows.
const maxRepository = 255

// ValidRepository reports whether name is a repository name a registry accepts, such as
// library/nginx. Its grammar rules out "..", so a valid name is also a safe relative path.
func ValidRepository(name string) bool {
	return len(name) <= maxRepository && repositoryPattern.MatchString(name)
}

// ValidTag reports whether tag is a tag a registry accepts, such as 1.27-alpine.
func ValidTag(tag string) bool { return tagPattern.MatchString(tag) }
