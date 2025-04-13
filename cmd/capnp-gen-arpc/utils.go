package main

import "strings"

func Capitalize(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func Uncapitalize(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}
